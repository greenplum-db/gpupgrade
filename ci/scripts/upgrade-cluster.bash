#!/bin/bash

set -eux -o pipefail

# Due to buggy RPATH settings and dependencies on LD_LIBRARY_PATH during our
# build process, it's very difficult to use the old and new binaries at the same
# time. (They end up cross-linked against each others' dependencies.)
# make_trampoline_directories() is a temporary workaround for this problem.
#
# It sets up a fake binary directory, for both the old and new clusters, that
# contains a set of symbolic links to the necessary executables. The trampoline
# binary will set PATH and LD_LIBRARY_PATH as if we had sourced greenplum_path,
# then call the actual executable with the provided arguments. This way we can
# avoid polluting the environment with either the old or new link paths; they're
# set just-in-time.
make_trampoline_directories() {
    cat - > /tmp/trampoline <<"EOF"
#! /bin/bash
set -eu -o pipefail

executable=$(basename ${BASH_SOURCE[0]})
gphome=$(readlink -e "$(dirname ${BASH_SOURCE[0]})/..")

if [ -z ${LD_LIBRARY_PATH+x} ]; then
    export LD_LIBRARY_PATH="${gphome}/lib:${gphome}/ext/python/lib"
else
    # Keep any existing paths around too.
    export LD_LIBRARY_PATH="${gphome}/lib:${gphome}/ext/python/lib:$LD_LIBRARY_PATH"
fi
export PATH="${gphome}/bin:$PATH"

"${gphome}/bin/${executable}" "$@"
EOF

    for host in "$@"; do
        scp /tmp/trampoline "$host":/tmp/trampoline

        time ssh centos@"$host" bash <<EOF
set -eux -o pipefail

sudo mkdir "${GPHOME_OLD}/fake-bin"
cd "${GPHOME_OLD}/fake-bin"
sudo cp /tmp/trampoline .
sudo chmod +x ./trampoline

sudo ln -s trampoline pg_controldata
sudo ln -s trampoline pg_ctl
sudo ln -s trampoline pg_resetxlog
sudo ln -s trampoline postgres

sudo ln -s trampoline gpstop

# GPHOME_NEW might be the same as GPHOME_OLD for same-version upgrades.
if [ "$GPHOME_NEW" != "$GPHOME_OLD" ]; then
    sudo mkdir "${GPHOME_NEW}/fake-bin"
    cd "${GPHOME_NEW}/fake-bin"
    sudo cp /tmp/trampoline .
    sudo chmod +x ./trampoline

    sudo ln -s trampoline pg_controldata
    sudo ln -s trampoline pg_ctl
    sudo ln -s trampoline pg_resetxlog
    sudo ln -s trampoline postgres

    sudo ln -s trampoline gpstop
fi

sudo ln -s trampoline initdb
sudo ln -s trampoline pg_dump
sudo ln -s trampoline pg_dumpall
sudo ln -s trampoline pg_restore
sudo ln -s trampoline pg_upgrade
sudo ln -s trampoline psql
sudo ln -s trampoline vacuumdb

sudo ln -s trampoline gpupgrade_agent # XXX this is silly
sudo ln -s trampoline gpinitsystem
sudo ln -s trampoline gpstart
EOF
    done
}

# We'll need this to transfer our built binaries over to the cluster hosts.
./ccp_src/scripts/setup_ssh_to_cluster.sh

# Cache our list of hosts to loop over below.
mapfile -t hosts < cluster_env_files/hostfile_all

# Copy over the SQL dump we pulled from master.
scp sqldump/dump.sql.xz gpadmin@mdw:/tmp/

# Build gpupgrade.
export GOPATH=$PWD/go
export PATH=$GOPATH/bin:$PATH

cd $GOPATH/src/github.com/greenplum-db/gpupgrade
make depend
make

# Install the artifacts onto the cluster machines.
artifacts='gpupgrade gpupgrade_hub gpupgrade_agent'
for host in "${hosts[@]}"; do
    scp $artifacts "gpadmin@$host:${GPHOME_NEW}/bin/"
done

echo 'Loading SQL dump into old cluster...'
time ssh mdw bash <<EOF
    set -eux -o pipefail

    source ${GPHOME_OLD}/greenplum_path.sh
    export PGOPTIONS='--client-min-messages=warning'
    unxz < /tmp/dump.sql.xz | psql -f - postgres
EOF

echo 'Creating fake binary directories for the environment...'
make_trampoline_directories "${hosts[@]}"

# Now do the upgrade.
time ssh mdw GPHOME_OLD="${GPHOME_OLD}" GPHOME_NEW="${GPHOME_NEW}" bash <<"EOF"
    set -eux -o pipefail

    wait_for_step() {
        local step="$1"
        local timeout=${2:-60} # default to 60 seconds
        local done=0

        for i in $(seq $timeout); do
            local output=$(gpupgrade status upgrade)
            if [ "$?" -ne "0" ]; then
                echo "$output"
                exit 1
            fi

            local status=$(grep "$step" <<< "$output")

            if [[ $status = *FAILED* ]]; then
                echo "$output"
                exit 1
            fi

            if [[ $status = *"COMPLETE - $step"* ]]; then
                done=1
                echo "$status"
                break
            fi

            sleep 1
        done

        if (( ! $done )); then
            echo "ERROR: timed out waiting for '${step}' to complete"
            exit 1
        fi
    }

    dump_sql() {
        local port=$1
        local dumpfile=$2

        echo "Dumping cluster contents from port ${port} to ${dumpfile}..."

        ssh -n mdw "
            source ${GPHOME_NEW}/greenplum_path.sh
            pg_dumpall -p ${port} -f '$dumpfile'
        "
    }

    compare_dumps() {
        local old_dump=$1
        local new_dump=$2

        echo "Comparing dumps at ${old_dump} and ${new_dump}..."

        ssh -n mdw "
            diff -U3 --speed-large-files --ignore-space-change '$old_dump' '$new_dump'
        "
    }

    dump_sql 5432 /tmp/old.sql

    # XXX gpupgrade needs to know where the hub is installed; see #117
    export PATH=${GPHOME_NEW}/bin:$PATH

    gpupgrade initialize \
              --new-bindir ${GPHOME_NEW}/fake-bin \
              --old-bindir ${GPHOME_OLD}/fake-bin \
              --old-port 5432

    gpupgrade prepare init-cluster
    wait_for_step "Initialize new cluster"

    gpupgrade prepare shutdown-clusters
    wait_for_step "Shutdown clusters"

    gpupgrade upgrade convert-master

    gpupgrade upgrade copy-master
    wait_for_step "Copy master data directory to segments"

    gpupgrade upgrade convert-primaries
    wait_for_step "Run pg_upgrade on primaries" 1200 # twenty minute timeout

    gpupgrade upgrade validate-start-cluster
    wait_for_step "Validate the upgraded cluster can start up"

    dump_sql 5433 /tmp/new.sql
    if ! compare_dumps /tmp/old.sql /tmp/new.sql; then
        echo 'error: before and after dumps differ'
        exit 1
    fi

    echo 'Upgrade successful.'
EOF
