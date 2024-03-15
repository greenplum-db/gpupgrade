-- Copyright (c) 2017-2024 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

--------------------------------------------------------------------------------
-- Create and setup non-upgradeable objects
--------------------------------------------------------------------------------

DROP SCHEMA IF EXISTS removed_functions CASCADE;
CREATE SCHEMA removed_functions;
SET search_path to removed_functions;

CREATE TABLE ao_table(i int) with (appendonly=true);
CREATE TABLE rank_table (id int, rank int, year int, gender
char(1), count int )
DISTRIBUTED BY (id)
PARTITION BY LIST (gender)
( PARTITION girls VALUES ('F'),
  PARTITION boys VALUES ('M'),
  DEFAULT PARTITION other );

CREATE VIEW v01 AS SELECT pg_catalog.pg_current_xlog_insert_location();
CREATE VIEW v02 AS SELECT pg_catalog.pg_current_xlog_location();
CREATE VIEW v03 AS SELECT pg_catalog.gp_update_ao_master_stats('ao_table'::regclass);
CREATE VIEW v04 AS SELECT pg_get_partition_def('rank_table'::regclass);
CREATE VIEW v05 AS SELECT pg_get_partition_def('rank_table'::regclass, true);
CREATE VIEW v06 AS SELECT pg_get_partition_def('rank_table'::regclass, true, true);
CREATE VIEW v07 AS SELECT pg_get_partition_rule_def('rank_table'::regclass);
CREATE VIEW v08 AS SELECT pg_get_partition_rule_def('rank_table'::regclass, true);
CREATE VIEW v09 AS SELECT pg_get_partition_template_def('rank_table'::regclass, true, true);
CREATE VIEW v10 AS SELECT pg_catalog.pg_is_xlog_replay_paused();
CREATE VIEW v11 AS SELECT pg_catalog.pg_last_xlog_receive_location();
CREATE VIEW v12 AS SELECT pg_catalog.pg_last_xlog_replay_location();
CREATE VIEW v13 AS SELECT pg_catalog.pg_switch_xlog();

---------------------------------------------------------------------------------
--- Assert that pg_upgrade --check correctly detects the non-upgradeable objects
---------------------------------------------------------------------------------
!\retcode gpupgrade initialize --source-gphome="${GPHOME_SOURCE}" --target-gphome=${GPHOME_TARGET} --source-master-port=${PGPORT} --disk-free-ratio 0 --non-interactive;
! find $(ls -dt ~/gpAdminLogs/gpupgrade/pg_upgrade_*/ | head -1) -name "views_with_removed_functions.txt" -exec cat {} +;

---------------------------------------------------------------------------------
--- Cleanup
---------------------------------------------------------------------------------
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
DROP TABLE rank_table;
DROP TABLE ao_table;
