-- Copyright (c) 2017-2024 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

--------------------------------------------------------------------------------
-- Create and setup non-upgradeable objects
--------------------------------------------------------------------------------

DROP SCHEMA IF EXISTS removed_types CASCADE;
CREATE SCHEMA removed_types;
SET search_path to removed_types;

CREATE VIEW v01 AS SELECT NULL::gp_toolkit.gp_size_of_partition_and_indexes_disk;
CREATE VIEW v02 AS SELECT NULL::gp_toolkit.__gp_user_data_tables;
CREATE VIEW v03 AS SELECT NULL::pg_catalog._abstime;
CREATE VIEW v04 AS SELECT NULL::pg_catalog.abstime;
CREATE VIEW v05 AS SELECT NULL::pg_catalog.pg_partition;
CREATE VIEW v06 AS SELECT NULL::pg_catalog.pg_partition_columns;
CREATE VIEW v07 AS SELECT NULL::pg_catalog.pg_partition_encoding;
CREATE VIEW v08 AS SELECT NULL::pg_catalog.pg_partition_rule;
CREATE VIEW v09 AS SELECT NULL::pg_catalog.pg_partitions;
CREATE VIEW v10 AS SELECT NULL::pg_catalog.pg_partition_templates;
CREATE VIEW v11 AS SELECT NULL::pg_catalog.pg_stat_partition_operations;
CREATE VIEW v12 AS SELECT NULL::pg_catalog._reltime;
CREATE VIEW v13 AS SELECT NULL::pg_catalog.reltime;
CREATE VIEW v14 AS SELECT NULL::pg_catalog.smgr;
CREATE VIEW v15 AS SELECT NULL::pg_catalog._tinterval;
CREATE VIEW v16 AS SELECT NULL::pg_catalog.tinterval;

---------------------------------------------------------------------------------
--- Assert that pg_upgrade --check correctly detects the non-upgradeable objects
---------------------------------------------------------------------------------
!\retcode gpupgrade initialize --source-gphome="${GPHOME_SOURCE}" --target-gphome=${GPHOME_TARGET} --source-master-port=${PGPORT} --disk-free-ratio 0 --non-interactive;
! cat ~/gpAdminLogs/gpupgrade/pg_upgrade/p-1/views_with_removed_types.txt;

---------------------------------------------------------------------------------
--- Cleanup
---------------------------------------------------------------------------------
DROP VIEW v16;
DROP VIEW v15;
DROP VIEW v14;
DROP VIEW v13;
DROP VIEW v12;
DROP VIEW v11;
DROP VIEW v10;
DROP VIEW v09;
DROP VIEW v08;
DROP VIEW v07;
DROP VIEW v06;
DROP VIEW v05;
DROP VIEW v04;
DROP VIEW v03;
DROP VIEW v02;
DROP VIEW v01;
