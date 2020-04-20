#! /bin/sh
#
# Copyright (c) 2017-2020 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

set -ex

apk add --no-progress openssh-client
cp -R cluster_env_files/.ssh /root/.ssh

scp sqldump/dump.sql.xz gpadmin@mdw:/tmp/

echo 'Loading SQL dump into source cluster...'
time ssh -n gpadmin@mdw "
    set -eux -o pipefail

    source /usr/local/greenplum-db-old/greenplum_path.sh
    export PGOPTIONS='--client-min-messages=warning'
    unxz < /tmp/dump.sql.xz | psql -f - postgres
"
