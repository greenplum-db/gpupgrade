#!/bin/bash
# Copyright (c) 2017-2020 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

cat << 'EOF'

SELECT current_database();

-- Cluster Statistics
SELECT hostname, COUNT(dbid) AS Primaries FROM pg_catalog.gp_segment_configuration WHERE role='p' GROUP BY hostname;
SELECT hostname, COUNT(dbid) AS Mirrors FROM pg_catalog.gp_segment_configuration WHERE role='m' GROUP BY hostname;

-- Extensions
SELECT COUNT(*) AS InstalledExtensions FROM pg_catalog.pg_extension;

-- Database size
SELECT pg_size_pretty(pg_database_size(current_database())) AS DatabaseSize;
SELECT COUNT(*) as Databases FROM pg_catalog.pg_database;

-- No. of triggers
SELECT COUNT(*) AS Triggers FROM pg_catalog.pg_trigger;

-- GUCs
SELECT COUNT(*) AS NonDefaultParameters FROM pg_catalog.pg_settings WHERE source <> 'default';

-- No. of Tablespaces
SELECT COUNT(*) AS Tablespaces FROM pg_catalog.pg_tablespace;

-- No. of schemas
SELECT COUNT(nspname) AS Schemas FROM pg_catalog.pg_namespace;

-- Table Statistics
SELECT COUNT(*) AS OrdinaryTables FROM pg_catalog.pg_class WHERE RELKIND='r';
SELECT COUNT(*) AS IndexTables FROM pg_catalog.pg_class WHERE RELKIND='i';
SELECT COUNT(*) AS Views FROM pg_catalog.pg_class WHERE RELKIND='v';
SELECT COUNT(*) AS ToastTables FROM pg_catalog.pg_class WHERE RELKIND='t';
SELECT COUNT(*) AS AOTables FROM pg_catalog.pg_appendonly WHERE columnstore = 'f';
SELECT COUNT(*) AS AOCOTables FROM pg_catalog.pg_appendonly WHERE columnstore = 't';
SELECT COUNT(*) AS UserTables FROM pg_catalog.pg_stat_user_tables;

-- No. of Columns in AOCO
SELECT COUNT(*) AS AOCOColumns FROM information_schema.columns
 WHERE table_name IN (SELECT relid::regclass::text FROM pg_catalog.pg_appendonly WHERE columnstore = 't');

-- Partition Table Statistics
SELECT COUNT(DISTINCT parrelid) AS RootPartitions FROM pg_catalog.pg_partition;
SELECT COUNT(DISTINCT parchildrelid) AS ChildPartitions FROM pg_catalog.pg_partition_rule;

-- No. of views
SELECT COUNT(*) AS Views FROM pg_catalog.pg_views;

-- No. of indexes
SELECT COUNT(*) AS Indexes FROM pg_catalog.pg_index;


EOF
