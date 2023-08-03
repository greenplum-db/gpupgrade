-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- Check for AO and AOCO parent partitions that contain entries in pg_aoseg
-- or pg_aocsseg respectively. These tables should not contain any entries
-- as parent partitions don't carry any data.
--
-- Running into such entries can cause unexpected failures during
-- pg_upgrade (and possibly during post-upgrade activity). One such ERROR
-- manifested when we were trying to run VACUUM FREEZE while upgrading
-- primaries. In this case, there was a root AOCO table with a non-empty
-- pg_aocsseg table on the master node.
--
--   "ERROR","58P01","could not open Append-Only segment file
--   ""base/16410/20863.1665"": No such file or directory",,,,,,"VACUUM
--     (FREEZE);",0,,"aomd.c"

--------------------------------------------------------------------------------
-- Create and setup non-upgradeable objects
--------------------------------------------------------------------------------

CREATE OR REPLACE FUNCTION insert_dummy_segentry(segrelfqname text)
    RETURNS void AS
$func$
BEGIN /* in func */
    EXECUTE 'INSERT INTO ' || segrelfqname || ' VALUES(null)'; /* in func */
END /* in func */
$func$  LANGUAGE plpgsql;

-- Test AO partition table
CREATE TABLE ao_root_partition (A INT, B INT) WITH (APPENDONLY=TRUE) DISTRIBUTED BY(A)
    PARTITION BY RANGE(A)
        SUBPARTITION BY RANGE(B)
        SUBPARTITION TEMPLATE (START(1) END (5) EVERY(1)) (START (1) END (2) EVERY (1));

INSERT INTO ao_root_partition SELECT 1,i FROM GENERATE_SERIES(1,4) AS i;
-- Create an artificial aoseg entry for the root and interior partition.
SET allow_system_table_mods TO DML;
SELECT insert_dummy_segentry(s.interior_segrelfqname) FROM
    (SELECT segrelid::regclass::text AS interior_segrelfqname FROM pg_appendonly
     WHERE relid IN ('ao_root_partition'::regclass, 'ao_root_partition_1_prt_1'::regclass)) AS s;
RESET allow_system_table_mods;

-- Test AOCO partition table
CREATE TABLE aoco_root_partition (A INT, B INT) WITH (APPENDONLY=TRUE, ORIENTATION=COLUMN) DISTRIBUTED BY(A)
    PARTITION BY RANGE(A)
        SUBPARTITION BY RANGE(B)
        SUBPARTITION TEMPLATE (START(1) END (5) EVERY(1)) (START (1) END (2) EVERY (1));

INSERT INTO aoco_root_partition SELECT 1,i FROM GENERATE_SERIES(1,4) AS i;
-- Create an artificial aocsseg entry for the root and interior partition.
SET allow_system_table_mods TO DML;
SELECT insert_dummy_segentry(s.interior_segrelfqname) FROM
    (SELECT segrelid::regclass::text AS interior_segrelfqname FROM pg_appendonly
     WHERE relid IN ('aoco_root_partition'::regclass, 'aoco_root_partition_1_prt_1'::regclass)) AS s;
RESET allow_system_table_mods;

--------------------------------------------------------------------------------
-- Assert that pg_upgrade --check correctly detects the non-upgradeable objects
--------------------------------------------------------------------------------
-- start_matchsubs
--
-- m/segrel pg_aoseg.pg_aoseg_\d+$/
-- s/segrel pg_aoseg.pg_aoseg_\d+/segrel pg_aoseg.pg_aoseg_XXXXX/
--
-- m/segrel pg_aoseg.pg_aocsseg_\d+$/
-- s/segrel pg_aoseg.pg_aocsseg_\d+/segrel pg_aoseg.pg_aocsseg_XXXXX/
--
-- end_matchsubs
!\retcode gpupgrade initialize --source-gphome="${GPHOME_SOURCE}" --target-gphome=${GPHOME_TARGET} --source-master-port=${PGPORT} --disk-free-ratio 0 --non-interactive;
! cat ~/gpAdminLogs/gpupgrade/pg_upgrade/p-1/parent_partitions_with_seg_entries.txt | LC_ALL=C sort -b;

--------------------------------------------------------------------------------
-- Workaround to unblock upgrade
--------------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION truncate_segrel(segrelfqname text)
    RETURNS void AS
$func$
BEGIN /* in func */
    EXECUTE 'DELETE FROM ' || segrelfqname; /* in func */
END /* in func */
$func$  LANGUAGE plpgsql;

SET allow_system_table_mods TO DML;

-- Truncate the artificial aoseg entries.
SELECT truncate_segrel(s.interior_segrelfqname) FROM
    (SELECT segrelid::regclass::text AS interior_segrelfqname FROM pg_appendonly
     WHERE relid IN ('ao_root_partition'::regclass, 'ao_root_partition_1_prt_1'::regclass, 'aoco_root_partition'::regclass, 'aoco_root_partition_1_prt_1'::regclass)) AS s;

RESET allow_system_table_mods;
