-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

--------------------------------------------------------------------------------
-- Create and setup non-upgradeable objects
--------------------------------------------------------------------------------

DROP SCHEMA IF EXISTS removed_operators CASCADE;
CREATE SCHEMA removed_operators;
SET search_path to removed_operators;

CREATE OR REPLACE VIEW view_with_int2vectoreq AS SELECT '1 2'::INT2VECTOR = '1 2'::INT2VECTOR;

---------------------------------------------------------------------------------
--- Assert that pg_upgrade --check correctly detects the non-upgradeable objects
---------------------------------------------------------------------------------
!\retcode gpupgrade initialize --source-gphome="${GPHOME_SOURCE}" --target-gphome=${GPHOME_TARGET} --source-master-port=${PGPORT} --disk-free-ratio 0 --non-interactive;
! find $(ls -dt ~/gpAdminLogs/gpupgrade/pg_upgrade_*/ | head -1) -name "views_with_removed_operators.txt" -exec cat {} +;

---------------------------------------------------------------------------------
--- Workaround to unblock upgrade
---------------------------------------------------------------------------------
DROP VIEW view_with_int2vectoreq;
