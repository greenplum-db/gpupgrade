-- Copyright (c) 2017-2021 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- An AO partition hierarchy having an index defined on the parent, that is
-- not defined on all of the members of the hierarchy, is non-upgradeable. Such
-- indexes must be dropped before an upgrade.

-- start_matchsubs
-- m/^Mismatched index on partition \d+ in relation \d+/
-- s/^Mismatched index on partition \d+ in relation \d+/^Mismatched index on partition ##### in relation #####/
-- end_matchsubs

--------------------------------------------------------------------------------
-- Create and setup non-upgradeable objects
--------------------------------------------------------------------------------
CREATE TABLE mismatched_partition_indexes (a integer, b text, c integer) WITH (appendonly=true) DISTRIBUTED BY (a) PARTITION BY RANGE(c) (START(1) END(3) EVERY(1));
CREATE INDEX mismatch_idx on mismatched_partition_indexes(b);

CREATE TABLE mismatch_exch (a integer, b text, c integer) WITH (appendonly=true) DISTRIBUTED BY (a);
ALTER TABLE mismatched_partition_indexes exchange partition for (rank(1)) with table mismatch_exch;

INSERT INTO mismatched_partition_indexes VALUES(1, 'apple', 1), (2, 'boss', 2);

--------------------------------------------------------------------------------
-- Assert that pg_upgrade --check correctly detects the non-upgradeable objects
--------------------------------------------------------------------------------
!\retcode gpupgrade initialize --source-gphome="${GPHOME_SOURCE}" --target-gphome=${GPHOME_TARGET} --source-master-port=${PGPORT} --disk-free-ratio 0 --automatic;
! cat ~/gpAdminLogs/gpupgrade/pg_upgrade/p-1/mismatched_aopartition_indexes.txt;

--------------------------------------------------------------------------------
-- Workaround to unblock upgrade
--------------------------------------------------------------------------------
DROP INDEX mismatch_idx_1_prt_2;
DROP INDEX mismatch_idx;
