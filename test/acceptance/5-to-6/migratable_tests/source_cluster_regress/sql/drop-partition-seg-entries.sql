-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

--------------------------------------------------------------------------------
-- Create and setup migratable objects
--------------------------------------------------------------------------------

CREATE OR REPLACE FUNCTION insert_dummy_segentry(segrelfqname text)
    RETURNS void AS
$func$
BEGIN
    EXECUTE 'INSERT INTO ' || segrelfqname || ' VALUES(null)'; /* in func */
END
$func$  LANGUAGE plpgsql;

-- test AO parent partition with seg entries
CREATE TABLE ao_root_partition (a int, b int) WITH (APPENDONLY=TRUE)
DISTRIBUTED BY (a)
PARTITION BY RANGE (a)
	SUBPARTITION BY RANGE (b)
	SUBPARTITION TEMPLATE (START(1) END (5) EVERY(1)) (START (1) END (2) EVERY (1));
INSERT INTO ao_root_partition VALUES(1, 1);
INSERT INTO ao_root_partition VALUES(1, 2);
INSERT INTO ao_root_partition VALUES(1, 3);

-- create an artificial aoseg entry for the root and interior partition.
SET allow_system_table_mods TO DML;
SELECT insert_dummy_segentry(s.interior_segrelfqname)
FROM (
    SELECT segrelid::regclass::text AS interior_segrelfqname
    FROM pg_appendonly
    WHERE relid IN ('ao_root_partition'::regclass, 'ao_root_partition_1_prt_1'::regclass)
) AS s;
RESET allow_system_table_mods;

-- test AOCO parent partition with seg entries
CREATE TABLE aoco_root_partition (a int, b int) WITH (APPENDONLY=TRUE, ORIENTATION=COLUMN)
DISTRIBUTED BY (a)
PARTITION BY RANGE (a)
    SUBPARTITION BY RANGE (b)
    SUBPARTITION TEMPLATE (START(1) END(5) EVERY(1)) (START(1) END(2) EVERY(1));
INSERT INTO aoco_root_partition VALUES(1, 1);
INSERT INTO aoco_root_partition VALUES(1, 2);
INSERT INTO aoco_root_partition VALUES(1, 3);

-- create an artificial aocsseg entry for the root and interior partition.
SET allow_system_table_mods TO DML;
SELECT insert_dummy_segentry(s.interior_segrelfqname)
FROM (
    SELECT segrelid::regclass::text AS interior_segrelfqname
    FROM pg_appendonly
    WHERE relid IN ('aoco_root_partition'::regclass, 'aoco_root_partition_1_prt_1'::regclass)
) AS s;
RESET allow_system_table_mods;

-- check data
SELECT * from ao_root_partition ORDER BY 1, 2;
SELECT * FROM aoco_root_partition ORDER BY 1, 2;
