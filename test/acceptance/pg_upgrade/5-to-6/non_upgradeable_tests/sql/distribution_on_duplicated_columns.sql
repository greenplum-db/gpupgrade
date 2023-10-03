-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- There is a bug in the GPDB5 parser that allows distribution on duplicated
-- columns. This has been patched in GPDB6+ so users need to fix any affected
-- table's distribution before upgrading. A check has been put in GPDB6 to
-- ensure upgrade will not continue if there are tables distributed on
-- duplicate columns.

--------------------------------------------------------------------------------
-- Create and setup non-upgradeable objects
--------------------------------------------------------------------------------
CREATE TABLE distributed_on_duplicated_columns1 (a int, b int) DISTRIBUTED BY (a, a, b);
CREATE TABLE distributed_on_duplicated_columns2 (a int, b int, c int) DISTRIBUTED BY (a, a, b, b, a, b, c);

--------------------------------------------------------------------------------
-- Assert that pg_upgrade --check correctly detects the non-upgradeable objects
--------------------------------------------------------------------------------
!\retcode gpupgrade initialize --source-gphome="${GPHOME_SOURCE}" --target-gphome=${GPHOME_TARGET} --source-master-port=${PGPORT} --disk-free-ratio 0 --non-interactive;
! cat ~/gpAdminLogs/gpupgrade/pg_upgrade/p-1/duplicate_column_distribution.txt;

--------------------------------------------------------------------------------
-- Workaround to unblock upgrade
--------------------------------------------------------------------------------
ALTER TABLE distributed_on_duplicated_columns1 SET WITH (REORGANIZE=TRUE) DISTRIBUTED BY (a, b);
ALTER TABLE distributed_on_duplicated_columns2 SET WITH (REORGANIZE=TRUE) DISTRIBUTED BY (a, b, c);
