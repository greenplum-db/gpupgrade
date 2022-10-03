#!/bin/bash
# Copyright (c) 2017-2022 VMware, Inc. or its affiliates
# SPDX-License-Identifier: Apache-2.0

cat << 'EOF'

SELECT current_database();

-- Extensions
SELECT COUNT(*) AS InstalledExtensions FROM pg_catalog.pg_extension;

-- Database Size
SELECT pg_size_pretty(pg_database_size(current_database())) AS DatabaseSize;
SELECT COUNT(*) as Databases FROM pg_catalog.pg_database;

-- No. of Triggers
SELECT COUNT(*) AS Triggers FROM pg_catalog.pg_trigger;

-- GUCs
SELECT COUNT(*) AS NonDefaultParameters FROM pg_catalog.pg_settings WHERE source <> 'default';

-- No. of Tablespaces
SELECT COUNT(*) AS Tablespaces FROM pg_catalog.pg_tablespace;

-- No. of Schemas
SELECT COUNT(nspname) AS Schemas FROM pg_catalog.pg_namespace;

-- Table Statistics
SELECT COUNT(*) AS OrdinaryTables FROM pg_catalog.pg_class WHERE RELKIND='r';
SELECT COUNT(*) AS IndexTables FROM pg_catalog.pg_class WHERE RELKIND='i';
SELECT COUNT(*) AS Views FROM pg_catalog.pg_class WHERE RELKIND='v';
SELECT COUNT(*) AS Sequences FROM pg_catalog.pg_class WHERE RELKIND='S';
SELECT COUNT(*) AS ToastTables FROM pg_catalog.pg_class WHERE RELKIND='t';
SELECT COUNT(*) AS AOTables FROM pg_catalog.pg_appendonly WHERE columnstore = 'f';
SELECT COUNT(*) AS AOCOTables FROM pg_catalog.pg_appendonly WHERE columnstore = 't';
SELECT COUNT(*) AS UserTables FROM pg_catalog.pg_stat_user_tables;
SELECT COUNT(*) AS ExternalTables FROM pg_catalog.pg_class WHERE RELSTORAGE = 'x';

-- No. of Columns in AOCO
SELECT COUNT(*) AS AOCOColumns FROM information_schema.columns
 WHERE table_name IN (SELECT relid::regclass::text FROM pg_catalog.pg_appendonly WHERE columnstore = 't');

-- Partition Table Statistics
SELECT COUNT(DISTINCT parrelid) AS RootPartitions FROM pg_catalog.pg_partition;
SELECT COUNT(DISTINCT parchildrelid) AS ChildPartitions FROM pg_catalog.pg_partition_rule;

-- No. of Views
SELECT COUNT(*) AS Views FROM pg_catalog.pg_views;

-- No. of Indexes
SELECT COUNT(*) AS Indexes FROM pg_catalog.pg_index;

-- No. of User Defined Functions
SELECT COUNT(*) FROM pg_proc p, pg_namespace n WHERE p.pronamespace = n.oid AND n.nspname NOT IN ('pg_catalog', 'information_schema', 'gp_toolkit');

EOF
