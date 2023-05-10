#! /bin/bash
# Copyright (c) 2017-2023 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

export GPHOME_SOURCE=/usr/local/greenplum-db-source
export GPHOME_TARGET=/usr/local/greenplum-db-target
export MASTER_DATA_DIRECTORY=/data/gpdata/coordinator/gpseg-1
export PGPORT=5432

echo "For functional testing pipeline enabling ssh to the ccp cluster..."
if [ -d saved_cluster_env_files ]; then
    tar -xzvf saved_cluster_env_files/cluster_env_files*.tar.gz
    cp -R cluster_env_files/.ssh /root/.ssh
fi
