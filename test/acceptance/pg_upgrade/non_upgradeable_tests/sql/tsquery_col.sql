-- Copyright (c) 2017-2021 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- GPDB5: The internal representation of tsquery columns have changed GPDB6
-- onwards. Thus, tables containing such columns are non-upgradeable. Such
-- tables need to have their tsquery columns altered to text before running an
-- upgrade.

--------------------------------------------------------------------------------
-- Create and setup non-upgradeable objects
--------------------------------------------------------------------------------
CREATE TABLE tsquery_tbl (a text, b tsquery) DISTRIBUTED BY (a);
INSERT INTO tsquery_tbl SELECT 'a', 'New&York' FROM generate_series(1,4);

--------------------------------------------------------------------------------
-- Assert that pg_upgrade --check correctly detects the non-upgradeable objects
--------------------------------------------------------------------------------
!\retcode gpupgrade initialize --source-gphome="${GPHOME_SOURCE}" --target-gphome=${GPHOME_TARGET} --source-master-port=${PGPORT} --disk-free-ratio 0 --automatic;
! cat ${GPUPGRADE_HOME}/pg_upgrade/seg-1/tables_using_tsquery.txt;

--------------------------------------------------------------------------------
-- Workaround to unblock upgrade
--------------------------------------------------------------------------------
ALTER TABLE tsquery_tbl ALTER COLUMN b TYPE TEXT;
