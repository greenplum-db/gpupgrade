-- Copyright (c) 2017-2021 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- GPDB5: In GPDB6, lead/lag functions with the second parameters as bigint are not
-- supported. Only such functions with integer as the second parameter are
-- supported. So, while upgrading, if there are such views schema restore will fail
-- during upgrade. Thus, such views are non-upgradeable. They must be dropped
-- before running an upgrade.

--------------------------------------------------------------------------------
-- Create and setup non-upgradeable objects
--------------------------------------------------------------------------------
CREATE TABLE lead_lag_tbl (a int, b text) DISTRIBUTED RANDOMLY;
INSERT INTO lead_lag_tbl SELECT 1, 'a';

CREATE VIEW lag_view_1 AS SELECT lag(b, 1::bigint, b) OVER (ORDER BY b) as lag FROM lead_lag_tbl;
CREATE VIEW lag_view_2 AS SELECT lag(b, 1::bigint) OVER (ORDER BY b) as lag FROM lead_lag_tbl;

CREATE VIEW lead_view_1 AS SELECT lead(b, 1::bigint, b) OVER (ORDER BY b) as lag FROM lead_lag_tbl;
CREATE VIEW lead_view_2 AS SELECT lead(b, 1::bigint) OVER (ORDER BY b) as lag FROM lead_lag_tbl;

--------------------------------------------------------------------------------
-- Assert that pg_upgrade --check correctly detects the non-upgradeable objects
--------------------------------------------------------------------------------
!\retcode gpupgrade initialize --source-gphome="${GPHOME_SOURCE}" --target-gphome=${GPHOME_TARGET} --source-master-port=${PGPORT} --disk-free-ratio 0 --automatic;
! cat ~/gpAdminLogs/gpupgrade/pg_upgrade/p-1/view_lead_lag_functions.txt | LC_ALL=C sort -b;

--------------------------------------------------------------------------------
-- Workaround to unblock upgrade
--------------------------------------------------------------------------------
DROP VIEW lag_view_1;
DROP VIEW lag_view_2;
DROP VIEW lead_view_1;
DROP VIEW lead_view_2;
