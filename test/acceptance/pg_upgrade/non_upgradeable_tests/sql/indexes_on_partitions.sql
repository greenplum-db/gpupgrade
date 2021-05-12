-- Copyright (c) 2017-2021 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- Indexes on partitioned table roots and child partitions are non-upgradebable.
-- These indexes must be dropped before running an upgrade.

--------------------------------------------------------------------------------
-- Create and setup non-upgradeable objects
--------------------------------------------------------------------------------

-- 1. Heap partition table
CREATE TABLE p_heap_table (id integer, first_name text) DISTRIBUTED BY (id) PARTITION BY RANGE(id) (START(1) END(3) EVERY(1));
INSERT INTO p_heap_table VALUES (1, 'Jane');
INSERT INTO p_heap_table VALUES (2, 'John');
CREATE INDEX p_heap_first_name_index ON p_heap_table(first_name);

-- 2. AO partition table
CREATE TABLE p_ao_table (id integer, first_name text) WITH (appendonly=true) DISTRIBUTED BY (id) PARTITION BY RANGE(id) (START(1) END(3) EVERY(1));
INSERT INTO p_ao_table VALUES (1, 'Jane');
INSERT INTO p_ao_table VALUES (2, 'John');
CREATE INDEX p_ao_first_name_index ON p_ao_table(first_name);

-- 3. AOCO partition table
CREATE TABLE p_aoco_table (id integer, first_name text) WITH (appendonly=true, orientation=column) DISTRIBUTED BY (id) PARTITION BY RANGE(id) (START(1) END(3) EVERY(1));
INSERT INTO p_aoco_table VALUES (1, 'Jane');
INSERT INTO p_aoco_table VALUES (2, 'John');
CREATE INDEX p_aoco_first_name_index ON p_aoco_table(first_name);

--------------------------------------------------------------------------------
-- Assert that pg_upgrade --check correctly detects the non-upgradeable objects
--------------------------------------------------------------------------------
!\retcode gpupgrade initialize --source-gphome="${GPHOME_SOURCE}" --target-gphome=${GPHOME_TARGET} --source-master-port=${PGPORT} --disk-free-ratio 0 --automatic;
! cat ${GPUPGRADE_HOME}/pg_upgrade/seg-1/partitioned_tables_indexes.txt | sort;

--------------------------------------------------------------------------------
-- Workaround to unblock upgrade
--------------------------------------------------------------------------------

-- 1. Heap partition table
DROP INDEX p_heap_first_name_index;
DROP INDEX p_heap_first_name_index_1_prt_1;
DROP INDEX p_heap_first_name_index_1_prt_2;

-- 2. AO partition table
DROP INDEX p_ao_first_name_index;
DROP INDEX p_ao_first_name_index_1_prt_1;
DROP INDEX p_ao_first_name_index_1_prt_2;

-- 3. AOCO partition table
DROP INDEX p_aoco_first_name_index;
DROP INDEX p_aoco_first_name_index_1_prt_1;
DROP INDEX p_aoco_first_name_index_1_prt_2;

