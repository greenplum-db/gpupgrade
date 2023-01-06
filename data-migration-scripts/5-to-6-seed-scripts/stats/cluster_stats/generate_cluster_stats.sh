#!/bin/bash
# Copyright (c) 2017-2023 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

cat << 'EOF'

-- Cluster Statistics
SELECT hostname, COUNT(dbid) AS Primaries FROM pg_catalog.gp_segment_configuration WHERE role='p' GROUP BY hostname;
SELECT hostname, COUNT(dbid) AS Mirrors FROM pg_catalog.gp_segment_configuration WHERE role='m' GROUP BY hostname;

EOF
