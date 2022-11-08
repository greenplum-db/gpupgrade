-- Copyright (c) 2017-2022 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- Alignment for the 'name' data type changed to 'char' in 8.4;
-- checks tables and indexes. So, if a name data type column is not the first
-- column in the table, the resultant alignment in the target cluster will be
-- incorrect. Thus, we consider such tables as non-upgradeable. Such columns
-- should have their type altered to varchar before running an upgrade.

--------------------------------------------------------------------------------
-- Create and setup non-upgradeable objects
--------------------------------------------------------------------------------
CREATE TABLE table_with_name_column(
	a int,
	a_name name
);

INSERT INTO table_with_name_column VALUES(1, 'abc def');

--------------------------------------------------------------------------------
-- Assert that pg_upgrade --check correctly detects the non-upgradeable objects
--------------------------------------------------------------------------------
!\retcode gpupgrade initialize --source-gphome="${GPHOME_SOURCE}" --target-gphome=${GPHOME_TARGET} --source-master-port=${PGPORT} --disk-free-ratio 0 --automatic;
! cat ~/gpAdminLogs/gpupgrade/pg_upgrade/p-1/tables_using_name.txt;

--------------------------------------------------------------------------------
-- Workaround to unblock upgrade
--------------------------------------------------------------------------------
ALTER TABLE table_with_name_column ALTER COLUMN a_name TYPE varchar;
