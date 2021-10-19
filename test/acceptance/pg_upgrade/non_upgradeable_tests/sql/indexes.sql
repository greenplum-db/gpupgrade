-- Copyright (c) 2017-2021 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- Invalid btree indexes are non-upgradeable. They must either be dropped or
-- reindexed (which will mark them as valid) before an upgrade.
--
-- GPDB5: It is hard to conceive a case where a btree index can be marked
-- invalid, since GPDB5 does not support the CREATE INDEX CONCURRENTLY
-- statement. So this test is a paranoid check for GPDB5 and more applicable to
-- major version upgrades from a future major version.

--------------------------------------------------------------------------------
-- Create and setup non-upgradeable objects
--------------------------------------------------------------------------------
CREATE TABLE tbl_with_btree_index(a int,b int);
CREATE INDEX btree_idx on  tbl_with_btree_index using btree(b);

-- mark index as invalid
SET allow_system_table_mods='dml';
UPDATE pg_index SET indisvalid = false WHERE indexrelid='btree_idx'::regclass;
RESET allow_system_table_mods;

--------------------------------------------------------------------------------
-- Assert that pg_upgrade --check correctly detects the non-upgradeable objects
--------------------------------------------------------------------------------
!\retcode gpupgrade initialize --source-gphome="${GPHOME_SOURCE}" --target-gphome=${GPHOME_TARGET} --source-master-port=${PGPORT} --disk-free-ratio 0 --automatic;
! cat ~/gpAdminLogs/gpupgrade/pg_upgrade/p-1/invalid_indexes.txt;

--------------------------------------------------------------------------------
-- Workaround to unblock upgrade
--------------------------------------------------------------------------------

-- reindex to mark the index as valid
REINDEX INDEX btree_idx;
