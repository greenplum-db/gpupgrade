-- Copyright (c) 2017-2021 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- Test to ensure that multiple flavors of partitioned heap tables can be
-- upgraded.

--------------------------------------------------------------------------------
-- Create and setup upgradeable objects
--------------------------------------------------------------------------------

--
-- partitioned heap table with data
--
CREATE TABLE p_basic (id INTEGER, name TEXT) DISTRIBUTED BY (id) PARTITION BY RANGE(id) (START(1) END(3) EVERY(1));
INSERT INTO p_basic VALUES (1, 'Jane');
INSERT INTO p_basic VALUES (2, 'John');

--
-- range partitioned heap table and add partitions with data
--
CREATE TABLE p_add_partition_test (a INT, b INT) PARTITION BY RANGE(B) (START(1) END(2));
INSERT INTO p_add_partition_test VALUES (1, 1);
INSERT INTO p_add_partition_test VALUES (2, 1);
-- add partition with a specific name
ALTER TABLE p_add_partition_test ADD PARTITION ADDED_PART START(2) END(3);
INSERT INTO p_add_partition_test VALUES (1, 2);
-- add partition with default name
ALTER TABLE p_add_partition_test ADD PARTITION START(3) END(4);
INSERT INTO p_add_partition_test VALUES (1, 3);

--
-- list partitioned heap table with add partitions with data
--
CREATE TABLE p_add_list_partition_test (a INT, b INT) PARTITION BY LIST(b) (PARTITION one VALUES (1));
INSERT INTO p_add_list_partition_test VALUES (1, 1);
INSERT INTO p_add_list_partition_test VALUES (2, 1);
-- add partition with a specific name
ALTER TABLE p_add_list_partition_test ADD PARTITION added_part VALUES(2);
INSERT INTO p_add_list_partition_test VALUES (1, 2);
-- add partition with default name
ALTER TABLE p_add_list_partition_test ADD PARTITION VALUES(3);
INSERT INTO p_add_list_partition_test VALUES (1, 3);

--
-- range partitioned heap table with default partition
--
CREATE TABLE p_split_partition_test (a INT, b INT) PARTITION BY RANGE(b) (START(1) END(2), DEFAULT PARTITION extra);
INSERT INTO p_split_partition_test SELECT i, i FROM generate_series(1,5)i;
ALTER TABLE p_split_partition_test SPLIT DEFAULT PARTITION START(2) END(5) INTO (PARTITION splitted, PARTITION extra);

--
-- partition heap table with sub-partitions
--
CREATE TABLE p_subpart_heap (id int, age int) DISTRIBUTED BY (id) PARTITION BY RANGE (id) SUBPARTITION BY RANGE (age) (PARTITION partition_id START(1) END(3) 
( SUBPARTITION subpartition_age_first START(1) END(20), SUBPARTITION subpartition_age_second START(20) END(30) ));
INSERT INTO p_subpart_heap (id, age) VALUES (1, 10), (2, 20);
VACUUM FREEZE;


--
-- partitioned table with a dropped column
--
CREATE TABLE dropped_column (a int, b int, c int, d int) DISTRIBUTED BY (c)
    PARTITION BY RANGE (a)
        (PARTITION part_1 START(1) END(5),
        PARTITION part_2 START(5));
INSERT INTO dropped_column SELECT i, i, i, i FROM generate_series(1, 10) i;
ALTER TABLE dropped_column DROP COLUMN d;
INSERT INTO dropped_column SELECT i, i, i FROM generate_series(10, 20) i;

--
-- partitioned table with the root partition has a dropped column reference but
-- none of its child partitions do.
--
CREATE TABLE root_has_dropped_column (a int, b int, c int, d int)
    PARTITION BY RANGE (a)
        (PARTITION part_1 START(1) END(5),
        PARTITION part_2 START(5));
INSERT INTO root_has_dropped_column SELECT i, i, i, i FROM generate_series(1, 10) i;
ALTER TABLE root_has_dropped_column DROP COLUMN d;

CREATE TABLE intermediate_table_1 (a int, b int, c int);
ALTER TABLE root_has_dropped_column EXCHANGE PARTITION part_1 WITH TABLE intermediate_table_1;
DROP TABLE intermediate_table_1;

CREATE TABLE intermediate_table_2 (a int, b int, c int);
ALTER TABLE root_has_dropped_column EXCHANGE PARTITION part_2 WITH TABLE intermediate_table_2;
DROP TABLE intermediate_table_2;

INSERT INTO root_has_dropped_column SELECT i, i, i FROM generate_series(10, 20) i;

--
-- partitioned table with a dropped and newly added column
--
CREATE TABLE dropped_and_added_column (a int, b int, c int, d numeric) DISTRIBUTED BY (a)
    PARTITION BY RANGE(c) SUBPARTITION BY range(d)
        (PARTITION part_1 START(0) END(42)
            (SUBPARTITION subpart_1 START(0) END(42)));

INSERT INTO dropped_and_added_column SELECT i, i, i, i FROM generate_series(1, 10) i;
ALTER TABLE dropped_and_added_column DROP COLUMN b;
ALTER TABLE dropped_and_added_column ADD COLUMN e int;
INSERT INTO dropped_and_added_column SELECT i, i, i, i FROM generate_series(10, 20) i;
