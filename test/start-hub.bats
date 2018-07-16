#! /usr/bin/env bats

# abort() is meant to be called from BATS tests. It will exit the process after
# printing its arguments to the TAP stream.
abort() {
    echo "# fatal: $@" 1>&3
    exit 1
}

# TODO: Killing every running hub is a bad idea. Implement a PID file and use
# that to kill the hub instead.
kill_hub() {
    pkill -9 gpupgrade_hub || true
    if ps -ef | grep -Gq "[g]pupgrade_hub"; then
        abort "didn't kill running hub"
    fi
}

setup() {
    kill_hub
}

teardown() {
    kill_hub
}

@test "start-hub finds the right hub binary and starts a daemonized process" {
    # The '3>&-' below is there because we must close fd 3 before forking new
    # processes in a BATS suite. For a full explanation, see
    #    https://github.com/bats-core/bats-core#file-descriptor-3-read-this-if-bats-hangs
    gpupgrade prepare start-hub 3>&-
    ps -ef | grep -Gq "[g]pupgrade_hub --daemon$"
}

@test "start-hub returns an error if the hub is already running" {
    gpupgrade prepare start-hub 3>&-

    # second start should return an error
    ! gpupgrade prepare start-hub
    # TODO: check for a useful error message
}

@test "start-hub does not return an error if an unrelated process has gpupgrade_hub in its name" {
    # Create a long-running process with gpupgrade_hub in the name.
    tmpdir=`mktemp -d`
    tmpfile="${tmpdir}/gpupgrade_hub_test_log"
    touch $tmpfile
    tail -f $tmpfile 3>&- & # we'll `kill %1` this process later
    ps -ef | grep -Gq "[g]pupgrade_hub" # double-check that it's actually there

    # Start the hub; there should be no errors.
    gpupgrade prepare start-hub 3>&-

    # Clean up.
    kill %1
    rm $tmpfile
    rm -r $tmpdir
}

@test "start-hub returns an error if gpupgrade_hub isn't on the PATH" {
    # Save the path to gpupgrade, since Bash can't look it up once we clear PATH
    GPUPGRADE=`which gpupgrade`

    ! PATH= $GPUPGRADE prepare start-hub
    # TODO: check for a useful error message
}
