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

-- 1. Differently aligned dropped column
CREATE TABLE p_different_aligned_column (a int, b aclitem, c int) DISTRIBUTED BY (a)
    PARTITION BY range(c)
        SUBPARTITION BY range(a) (PARTITION p_part_with_different_alignedd_dropped_columns START(0) END(42)
            (SUBPARTITION subpart_differnt_aligned_column START(0) END(22),
            SUBPARTITION p_subpart_with_different_alignedd_dropped_columns START(22) END(42)));

-- 'b' column is intentionally differently aligned - aclitem has 'i' alignment
-- and timetz has 'd' alignment. If we allow the upgrade then on the new cluster
-- we will fetch column 'c' at the wrong offset.
CREATE TABLE subpart_different_aligned_column(a int, b timetz, c int);
ALTER TABLE p_different_aligned_column DROP COLUMN b;
INSERT INTO subpart_different_aligned_column VALUES (1, '00:00:00-8', 1), (2, '00:00:00-8', 2);
ALTER TABLE subpart_different_aligned_column DROP COLUMN b;
INSERT INTO p_different_aligned_column VALUES(22, 22), (23, 23);
ALTER TABLE p_different_aligned_column ALTER PARTITION p_part_with_different_alignedd_dropped_columns
    EXCHANGE PARTITION subpart_differnt_aligned_column WITH TABLE subpart_different_aligned_column;

-- 2. Differently aligned dropped varlena column
CREATE TABLE p_diff_aligned_varlena (a int, b float8[], c int) DISTRIBUTED BY (a)
    PARTITION BY range(c)
        SUBPARTITION BY range(a) (PARTITION varlena START(0) END(42)
            (SUBPARTITION varlena_first START(0) END(22),
            SUBPARTITION varlena_second START(22) END(42)));

-- 'b' column is intentionally differently aligned - float8[] has 'd'
-- alignment and numeric has 'i' alignment. If we allow the upgrade then on
-- the new cluster we will fetch column 'c' at the wrong offset.
CREATE TABLE varlena_first(a int, b numeric, c int);
ALTER TABLE p_diff_aligned_varlena DROP COLUMN b;
INSERT INTO varlena_first VALUES (1, 1.987654321, 1), (2, 2.3456789, 2);
ALTER TABLE varlena_first DROP COLUMN b;
ALTER TABLE p_diff_aligned_varlena ALTER PARTITION varlena EXCHANGE PARTITION varlena_first WITH TABLE varlena_first;

-- 3. Differently sized dropped column
CREATE TABLE p_different_size_column (a int, b int, c int) DISTRIBUTED BY (a)
    PARTITION BY range(c)
        SUBPARTITION BY range(a) (PARTITION p_part_with_different_sized_dropped_columns START(0) END(42)
            (SUBPARTITION subpart_differnt_size_column START(0) END(22),
            SUBPARTITION p_subpart_with_different_sized_dropped_columns START(22) END(42)));

CREATE TABLE subpart_different_size_column(a int, b numeric, c int);
ALTER TABLE p_different_size_column DROP COLUMN b;
ALTER TABLE subpart_different_size_column DROP COLUMN b;
ALTER TABLE p_different_size_column ALTER PARTITION p_part_with_different_sized_dropped_columns EXCHANGE PARTITION subpart_differnt_size_column WITH TABLE subpart_different_size_column;

--------------------------------------------------------------------------------
-- Assert that pg_upgrade --check correctly detects the non-upgradeable objects
--------------------------------------------------------------------------------
!\retcode gpupgrade initialize --source-gphome="${GPHOME_SOURCE}" --target-gphome=${GPHOME_TARGET} --source-master-port=${PGPORT} --disk-free-ratio 0 --automatic;
! cat ${GPUPGRADE_HOME}/pg_upgrade/seg-1/heterogeneous_partitioned_tables.txt | sort -b -d;

--------------------------------------------------------------------------------
-- Workaround to unblock upgrade
--------------------------------------------------------------------------------

-- 1. Differently aligned dropped column
ALTER TABLE p_different_aligned_column RENAME TO p_with_different_aligned_dropped_columns_broken;
CREATE TABLE p_different_aligned_column AS SELECT * FROM p_with_different_aligned_dropped_columns_broken;
DROP TABLE p_with_different_aligned_dropped_columns_broken CASCADE;

-- 2. Differently aligned dropped varlena column
-- TODO: Add steps to recreate the table as a workaround
-- Drop the table for now
DROP TABLE p_diff_aligned_varlena;

-- 3. Differently sized dropped column
ALTER TABLE p_different_size_column RENAME TO p_with_different_size_dropped_columns_broken;
CREATE TABLE p_different_size_column AS SELECT * FROM p_with_different_size_dropped_columns_broken;
DROP TABLE p_with_different_size_dropped_columns_broken CASCADE;
