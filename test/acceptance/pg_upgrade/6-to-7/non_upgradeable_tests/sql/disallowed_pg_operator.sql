-- Copyright (c) 2017-2024 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

--------------------------------------------------------------------------------
-- Create and setup non-upgradeable objects
--------------------------------------------------------------------------------
CREATE OPERATOR => (leftarg = int8, procedure = numeric_fac);

CREATE DATABASE test_disallowed_pg_operator;
1:@db_name test_disallowed_pg_operator:CREATE OPERATOR => (leftarg = int8, procedure = numeric_fac);
1q:

---------------------------------------------------------------------------------
--- Assert that pg_upgrade --check correctly detects the non-upgradeable objects
---------------------------------------------------------------------------------
!\retcode gpupgrade initialize --source-gphome="${GPHOME_SOURCE}" --target-gphome=${GPHOME_TARGET} --source-master-port=${PGPORT} --disk-free-ratio 0 --non-interactive;
! find $(ls -dt ~/gpAdminLogs/gpupgrade/pg_upgrade_*/ | head -1) -name "databases_with_disallowed_pg_operator.txt" -exec cat {} +;

---------------------------------------------------------------------------------
--- Cleanup
---------------------------------------------------------------------------------

DROP OPERATOR => (bigint, NONE);
DROP DATABASE test_disallowed_pg_operator;
