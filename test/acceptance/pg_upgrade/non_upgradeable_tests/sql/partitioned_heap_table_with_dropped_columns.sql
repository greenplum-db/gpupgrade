-- Copyright (c) 2017-2021 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- Heterogeneous partitions hierarchies arise when the on-disk representation of
-- the root partition doesn't match the on-disk representation of one or more
-- children. This can arise with dropped columns. When the dropped column type is
-- differently aligned or of different length, then after an upgrade we may read
-- from wrong offsets in the data file. Thus, we consider such hierarchies as
-- non-upgradeable. Such tables need to have their data dumped, dropped and
-- recreated with their original schema, and repopulated, before running an
-- upgrade.

--------------------------------------------------------------------------------
-- Create and setup non-upgradeable objects
--------------------------------------------------------------------------------

-- 1. Heterogeneous partition table with dropped column
--    The root and only a subset of children have the dropped column reference.
CREATE TABLE dropped_column (a int, b int, c char, d varchar(50)) DISTRIBUTED BY (c)
    PARTITION BY RANGE (a)
        (PARTITION part_1 START(1) END(5),
        PARTITION part_2 START(5));
ALTER TABLE dropped_column DROP COLUMN d;

-- Splitting the subpartition leads to its rewrite, eliminating its dropped column
-- reference. So, after this, only part_2 and the root partition will have a
-- dropped column reference.
ALTER TABLE dropped_column SPLIT PARTITION FOR(1) AT (2) INTO (PARTITION split_part_1, PARTITION split_part_2);
INSERT INTO dropped_column VALUES(1, 2, 'a');

-- 2. Root partitions do not have dropped column references, but some child partitions do
CREATE TABLE child_has_dropped_column (a int, b int, c char, d varchar(50))
    PARTITION BY RANGE (a)
        (PARTITION part_1 START(1) END(5),
        PARTITION part_2 START(5));

CREATE TABLE intermediate_table (a int, b int, c char, d varchar(50), to_drop int);
ALTER TABLE intermediate_table DROP COLUMN to_drop;

ALTER TABLE child_has_dropped_column EXCHANGE PARTITION part_1 WITH TABLE intermediate_table;
DROP TABLE intermediate_table;

-- 3. Root and child partitions have different number of dropped column references
CREATE TABLE diff_no_dropped_columns (a int, b int, c char, to_drop varchar(50))
    PARTITION BY RANGE (a)
        (PARTITION part_1 START(1) END(5),
        PARTITION part_2 START(5));
ALTER TABLE diff_no_dropped_columns DROP COLUMN to_drop;

CREATE TABLE intermediate_table (a int, b int, c char, to_drop varchar(50), to_drop_2 int);
ALTER TABLE intermediate_table DROP COLUMN to_drop;
ALTER TABLE intermediate_table DROP COLUMN to_drop_2;

ALTER TABLE diff_no_dropped_columns EXCHANGE PARTITION part_1 WITH TABLE intermediate_table;
DROP TABLE intermediate_table;

-- 4. Differently aligned dropped column
CREATE TABLE differently_aligned_column (a int, b aclitem, c int) DISTRIBUTED BY (a)
    PARTITION BY range(c)
        SUBPARTITION BY range(a) (PARTITION differently_aligned_columns_part START(0) END(42)
            (SUBPARTITION subpart_1 START(0) END(22),
            SUBPARTITION subpart_2 START(22) END(42)));
ALTER TABLE differently_aligned_column DROP COLUMN b;
INSERT INTO differently_aligned_column VALUES(22, 22), (23, 23);

-- 'b' column is intentionally differently aligned - aclitem has 'i' alignment
-- and timetz has 'd' alignment. If we allow the upgrade then on the new cluster
-- we will fetch column 'c' at the wrong offset.
CREATE TABLE intermediate_table (a int, b timetz, c int);
INSERT INTO intermediate_table VALUES (1, '00:00:00-8', 1), (2, '00:00:00-8', 2);
ALTER TABLE intermediate_table DROP COLUMN b;

ALTER TABLE differently_aligned_column ALTER PARTITION differently_aligned_columns_part
    EXCHANGE PARTITION subpart_1 WITH TABLE intermediate_table;
DROP TABLE intermediate_table;

-- 5. Differently aligned dropped varlena column
CREATE TABLE differently_aligned_varlena (a int, b float8[], c int) DISTRIBUTED BY (a)
    PARTITION BY range(c)
        SUBPARTITION BY range(a) (PARTITION differently_aligned_varlena_part START(0) END(42)
            (SUBPARTITION subpart_1 START(0) END(22),
            SUBPARTITION subpart_2 START(22) END(42)));
ALTER TABLE differently_aligned_varlena DROP COLUMN b;

-- 'b' column is intentionally differently aligned - float8[] has 'd'
-- alignment and numeric has 'i' alignment. If we allow the upgrade then on
-- the new cluster we will fetch column 'c' at the wrong offset.
CREATE TABLE intermediate_table(a int, b numeric, c int);
INSERT INTO intermediate_table VALUES (1, 1.987654321, 1), (2, 2.3456789, 2);
ALTER TABLE intermediate_table DROP COLUMN b;

ALTER TABLE differently_aligned_varlena ALTER PARTITION differently_aligned_varlena_part
    EXCHANGE PARTITION subpart_1 WITH TABLE intermediate_table;
DROP TABLE intermediate_table;

-- 6. Differently sized dropped column
CREATE TABLE differently_sized_column (a int, b int, c int) DISTRIBUTED BY (a)
    PARTITION BY range(c)
        SUBPARTITION BY range(a) (PARTITION differently_sized_column_part START(0) END(42)
            (SUBPARTITION subpart_1 START(0) END(22),
            SUBPARTITION subpart_2 START(22) END(42)));
ALTER TABLE differently_sized_column DROP COLUMN b;

CREATE TABLE intermediate_table(a int, b numeric, c int);
ALTER TABLE intermediate_table DROP COLUMN b;

ALTER TABLE differently_sized_column ALTER PARTITION differently_sized_column_part
    EXCHANGE PARTITION subpart_1 WITH TABLE intermediate_table;
DROP TABLE intermediate_table;

-- 7. Child having a different column order than the root
-- At the end of the scenario the root will have cols (a, b, ..dropped) and part_1 will have cols (a, ..dropped, b)
CREATE TABLE dropped_cols_out_of_order (a int, b int, to_drop int)
    PARTITION BY RANGE (a)
        (PARTITION part_1 START(1) END(5),
        PARTITION part_2 START(5));
ALTER TABLE dropped_cols_out_of_order DROP COLUMN to_drop;

CREATE TABLE intermediate_table(a int, to_drop int);
ALTER TABLE intermediate_table DROP COLUMN to_drop;
ALTER TABLE intermediate_table ADD COLUMN b int;

ALTER TABLE dropped_cols_out_of_order EXCHANGE PARTITION part_1 WITH TABLE intermediate_table;
DROP TABLE intermediate_table;

--------------------------------------------------------------------------------
-- Assert that pg_upgrade --check correctly detects the non-upgradeable objects
--------------------------------------------------------------------------------
!\retcode gpupgrade initialize --source-gphome="${GPHOME_SOURCE}" --target-gphome=${GPHOME_TARGET} --source-master-port=${PGPORT} --disk-free-ratio 0 --automatic;
-- NOTE: We sort the output to ensure the test is deterministic. See commit b6a084c. However, this prevents asserting
-- the correct tables were detected for the sub-checks "invalid dropped column references" and "misaligned columns".
-- Thus, we split the file and sort the two sub-checks individually.
! csplit -f parts ~/gpAdminLogs/gpupgrade/pg_upgrade/p-1/heterogeneous_partitioned_tables.txt '/Partitions with misaligned dropped column references:/';
! cat parts00 | LC_ALL=C sort -b;
! cat parts01 | LC_ALL=C sort -b;

--------------------------------------------------------------------------------
-- Workaround to unblock upgrade
--------------------------------------------------------------------------------

-- 1. Heterogeneous partition table with dropped column
--    The root and only a subset of children have the dropped column reference.
-- Perform a CTAS on only the affected partitions.
-- The root and sub-root partitions do not have any data and will be ignored
-- by the pg_upgrade check.
CREATE TABLE scratch_table AS SELECT * FROM dropped_column_1_prt_part_2;
BEGIN;
ALTER TABLE dropped_column EXCHANGE PARTITION part_2 WITH TABLE scratch_table;
COMMIT;
DROP TABLE scratch_table;

-- 2. Root partitions do not have dropped column references, but some child partitions do
-- Perform a CTAS on only the affected partitions.
-- The root and sub-root partitions do not have any data and will be ignored
-- by the pg_upgrade check.
CREATE TABLE scratch_table AS SELECT * FROM child_has_dropped_column_1_prt_part_1;
BEGIN;
ALTER TABLE child_has_dropped_column EXCHANGE PARTITION part_1 WITH TABLE scratch_table;
COMMIT;
DROP TABLE scratch_table;

-- 3. Root and child partitions have different number of dropped column references
-- Perform a CTAS on only the affected partitions.
-- The root and sub-root partitions do not have any data and will be ignored
-- by the pg_upgrade check.
CREATE TABLE scratch_table_part_1 AS SELECT * FROM diff_no_dropped_columns_1_prt_part_1;
CREATE TABLE scratch_table_part_2 AS SELECT * FROM diff_no_dropped_columns_1_prt_part_2;
BEGIN;
ALTER TABLE diff_no_dropped_columns EXCHANGE PARTITION part_1 WITH TABLE scratch_table_part_1;
ALTER TABLE diff_no_dropped_columns EXCHANGE PARTITION part_2 WITH TABLE scratch_table_part_2;
COMMIT;
DROP TABLE scratch_table_part_1;
DROP TABLE scratch_table_part_2;

-- 4. Differently aligned dropped column
-- Fix the affected partitions by recreating them with the proper dropped column references.
CREATE TABLE scratch_table_subpart_1 (a int, b aclitem, c int) DISTRIBUTED BY (a);
ALTER TABLE scratch_table_subpart_1 DROP COLUMN b;
INSERT INTO scratch_table_subpart_1 SELECT * FROM differently_aligned_colu_1_prt_differently_alig_2_prt_subpart_1;
BEGIN;
ALTER TABLE differently_aligned_column ALTER PARTITION differently_aligned_columns_part EXCHANGE PARTITION subpart_1 WITH TABLE scratch_table_subpart_1;
COMMIT;
DROP TABLE scratch_table_subpart_1;

-- 5. Differently aligned dropped varlena column
-- Show an alternative way of fixing the affected partitions by performing a
-- CTAS on all child partitions.
-- The root and sub-root partitions do not have any data and will be ignored
-- by the pg_upgrade check.
CREATE TABLE scratch_table_subpart_1 AS SELECT * FROM differently_aligned_varl_1_prt_differently_alig_2_prt_subpart_1;
CREATE TABLE scratch_table_subpart_2 AS SELECT * FROM differently_aligned_varl_1_prt_differently_alig_2_prt_subpart_2;
BEGIN;
ALTER TABLE differently_aligned_varlena ALTER PARTITION differently_aligned_varlena_part EXCHANGE PARTITION subpart_1 WITH TABLE scratch_table_subpart_1;
ALTER TABLE differently_aligned_varlena ALTER PARTITION differently_aligned_varlena_part EXCHANGE PARTITION subpart_2 WITH TABLE scratch_table_subpart_2;
COMMIT;
DROP TABLE scratch_table_subpart_1;
DROP TABLE scratch_table_subpart_2;

-- 6. Differently sized dropped column
-- Fix the affected partitions by recreating them with the proper dropped column references.
CREATE TABLE scratch_table_subpart_1 (a int, b int, c int) DISTRIBUTED BY (a);
ALTER TABLE scratch_table_subpart_1 DROP COLUMN b;
INSERT INTO scratch_table_subpart_1 SELECT * FROM differently_sized_column_1_prt_differently_size_2_prt_subpart_1;
BEGIN;
ALTER TABLE differently_sized_column ALTER PARTITION differently_sized_column_part EXCHANGE PARTITION subpart_1 WITH TABLE scratch_table_subpart_1;
COMMIT;
DROP TABLE scratch_table_subpart_1;

-- 7. Child having a different column order than the root
-- Fix the affected partitions by recreating them with the proper dropped column order.
CREATE TABLE scratch_table (a int, b int, to_drop int);
ALTER TABLE scratch_table DROP COLUMN to_drop;
INSERT INTO scratch_table SELECT * FROM dropped_cols_out_of_order_1_prt_part_1;
BEGIN;
ALTER TABLE dropped_cols_out_of_order EXCHANGE PARTITION part_1 WITH TABLE scratch_table;
COMMIT;
DROP TABLE scratch_table;

-- To fix the entire partition table there are two options:
-- 1) using gpbackup and gprestore, or 2) using pg_dump.
--------------------------------------------------------------------------------
-- To fix the entire table using gpbackup and gprestore:
--------------------------------------------------------------------------------
-- gpbackup --metadata-only --dbname postgres --include-table user_schema.table_part
--
-- Record the form the output above "Backup Timestamp = 20220126161009"
--
-- CREATE SCHEMA scratch;
--
-- gprestore --timestamp 20220126161009 --redirect-schema scratch --include-table user_schema.table_part
--
-- INSERT INTO scratch.table_part SELECT * FROM user_schema.table_part;
--
-- BEGIN;
-- DROP TABLE user_schema.table_part;
--
-- ALTER TABLE scratch.table_part SET SCHEMA user_schema;
-- ALTER TABLE scratch.table_part_1_prt_subpart SET SCHEMA user_schema;
-- ALTER TABLE scratch.table_part_1_prt_subpart_2_prt_subpart_1 SET SCHEMA user_schema;
-- ALTER TABLE scratch.table_part_1_prt_subpart_2_prt_subpart_2 SET SCHEMA user_schema;
-- COMMIT;
--
-- DROP SCHEMA scratch;
--------------------------------------------------------------------------------
-- To fix the entire table using pg_dump:
--------------------------------------------------------------------------------
-- pg_dump --gp-syntax  --schema-only -t user_schema.table_part postgres > out.sql
--
-- Edit out.sql and update all object references to use the "scratch" schema name.
-- For example, "CREATE TABLE scratch.table_part ..."
--
-- CREATE SCHEMA scratch;
-- psql -d postgres -f out.sql
--
-- INSERT INTO scratch.table_part SELECT * FROM user_schema.table_part;
--
-- BEGIN;
-- DROP TABLE user_schema.table_part;
--
-- ALTER TABLE scratch.table_part SET SCHEMA user_schema;
-- ALTER TABLE scratch.table_part_1_prt_subpart SET SCHEMA user_schema;
-- ALTER TABLE scratch.table_part_1_prt_subpart_2_prt_subpart_1 SET SCHEMA user_schema;
-- ALTER TABLE scratch.table_part_1_prt_subpart_2_prt_subpart_2 SET SCHEMA user_schema;
-- COMMIT;
--
-- DROP SCHEMA scratch;
--------------------------------------------------------------------------------
