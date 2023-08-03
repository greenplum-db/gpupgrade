-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- Ensure no pg_aoseg entries are created for AO or AOCO tables when there are
-- no tuples to add. That is, only create a pg_aoseg entry when the tupcount is
-- greater than 0. This ensures post-upgrade vacuum freeze succeeds. Entries can
-- be created when there are no tuples to add when altering the distribution of
-- an AOCO partition table which does CREATE and INSERT across, or when
-- inserting an empty row.
--
-- Having empty pg_aoseg entries caused pg_upgrade vacuum freeze errors.
-- For example, while upgrading the primaries a VACUUM FREEZE on a root AOCO
-- table with a non-empty pg_aocsseg table on the master resulted in:
--  "ERROR","58P01","could not open Append-Only segment file
--  ""base/16410/20863.1665"": No such file or directory",,,,,,"VACUUM
--    (FREEZE);",0,,"aomd.c"

--------------------------------------------------------------------------------
-- Create and setup upgradeable objects
--------------------------------------------------------------------------------
CREATE OR REPLACE FUNCTION truncate_segrel(segrelname text)
    RETURNS void AS
$func$
BEGIN /* in func */
EXECUTE 'DELETE FROM ' || segrelname; /* in func */
END /* in func */
$func$  LANGUAGE plpgsql;

-- Scenario 1a: Alter Distribution of AO partition table
CREATE TABLE alter_dist_key_for_ao_partition_table (a integer, b text, c integer)
    WITH (APPENDONLY=TRUE) DISTRIBUTED BY (a)
    PARTITION BY RANGE(c) (START(1) END(3) EVERY(1));
ALTER TABLE alter_dist_key_for_ao_partition_table SET DISTRIBUTED BY (b);

-- assert non-empty seg entries
SELECT * FROM gp_toolkit.__gp_aoseg_name('alter_dist_key_for_ao_partition_table');

-- truncate pg_aocsseg entries so pg_upgrade --check passes
SET allow_system_table_mods TO DML;
SELECT truncate_segrel(segrelid::regclass::text)
FROM pg_appendonly  WHERE relid = 'alter_dist_key_for_ao_partition_table'::regclass;
RESET allow_system_table_mods;


-- Scenario 1b: Alter Distribution of AOCO partition table
CREATE TABLE alter_dist_key_for_aoco_partition_table (a integer, b text, c integer)
    WITH (APPENDONLY=TRUE, ORIENTATION=COLUMN) DISTRIBUTED BY (a)
    PARTITION BY RANGE(c) (START(1) END(3) EVERY(1));
ALTER TABLE alter_dist_key_for_aoco_partition_table SET DISTRIBUTED BY (b);

-- assert non-empty seg entries
SELECT * FROM gp_toolkit.__gp_aocsseg_name('alter_dist_key_for_aoco_partition_table');

-- truncate pg_aocsseg entries so pg_upgrade --check passes
SET allow_system_table_mods TO DML;
SELECT truncate_segrel(segrelid::regclass::text)
    FROM pg_appendonly  WHERE relid = 'alter_dist_key_for_aoco_partition_table'::regclass;
RESET allow_system_table_mods;


-- Scenario 2a: Inserting an empty row into AO table
CREATE table ao_insert_empty_row (a integer, b text, c integer) WITH (APPENDONLY=TRUE) DISTRIBUTED BY (a);
INSERT INTO ao_insert_empty_row SELECT 1,'a',1 FROM gp_id WHERE dbid=-999;

-- assert non-empty seg entry
SELECT * FROM gp_toolkit.__gp_aoseg_name('ao_insert_empty_row');

-- truncate pg_aoseg entries so pg_upgrade --check passes
SET allow_system_table_mods TO DML;
SELECT truncate_segrel(segrelid::regclass::text)
    FROM pg_appendonly  WHERE relid = 'ao_insert_empty_row'::regclass;
RESET allow_system_table_mods;


-- Scenario 2b: Inserting an empty row into AOCO table
CREATE table aoco_insert_empty_row (a integer, b text, c integer) WITH (APPENDONLY=TRUE, ORIENTATION=COLUMN) DISTRIBUTED BY (a);
INSERT INTO aoco_insert_empty_row SELECT 1,'a',1 FROM gp_id WHERE dbid=-999;

-- assert non-empty seg entry
SELECT * FROM gp_toolkit.__gp_aocsseg_name('aoco_insert_empty_row');

-- truncate pg_aocsseg entries so pg_upgrade --check passes
SET allow_system_table_mods TO DML;
SELECT truncate_segrel(segrelid::regclass::text)
FROM pg_appendonly  WHERE relid = 'aoco_insert_empty_row'::regclass;
RESET allow_system_table_mods;
