-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

--------------------------------------------------------------------------------
-- Create and setup non-upgradeable objects
--------------------------------------------------------------------------------

-- The query that looks for these types had to be rewritten for 6 > 7 upgrade
-- because the recursive query looking for these types of relations contained a
-- self reference in a subquery. This specific type of query is disabled in 6x
-- so it was rewritten in plpgsql.

-- Data type 'unknown' is no longer allowed in table columns

CREATE table unknown_test (v varchar(20), n numeric(20, 2), t timestamp(2));

CREATE DOMAIN domain_using_unknown AS unknown;
CREATE TABLE table_using_unknown (
	col0 int,
	col1 unknown,
	col2 domain_using_unknown
);

-- build custom types that depend on each other to test recursive query used to
-- find the tables that depend on unknown types.
CREATE TYPE unknown_type AS (
	t0 unknown
);
CREATE TYPE arr_unknown_type1 AS (
	t1 unknown_type[]
);
CREATE TYPE arr_unknown_type2 AS (
	t2 arr_unknown_type1[]
);
CREATE TYPE arr_unknown_type3 AS (
	t3 arr_unknown_type2[]
);
CREATE TABLE table_using_multiple_layers_of_unknown_type (
    col0 int,
    col1 unknown_type,
    col2 arr_unknown_type1,
    col3 arr_unknown_type2,
    col4 arr_unknown_type3
);

---------------------------------------------------------------------------------
--- Assert that pg_upgrade --check correctly detects the non-upgradeable objects
---------------------------------------------------------------------------------
!\retcode gpupgrade initialize --source-gphome="${GPHOME_SOURCE}" --target-gphome=${GPHOME_TARGET} --source-master-port=${PGPORT} --disk-free-ratio 0 --non-interactive;
! find $(ls -dt ~/gpAdminLogs/gpupgrade/pg_upgrade_*/ | head -1) -name "tables_using_unknown.txt" -exec cat {} +;

---------------------------------------------------------------------------------
--- Workaround to unblock upgrade
---------------------------------------------------------------------------------
DROP TABLE table_using_multiple_layers_of_unknown_type;
DROP TABLE table_using_unknown;

DROP TYPE arr_unknown_type3;
DROP TYPE arr_unknown_type2;
DROP TYPE arr_unknown_type1;
DROP TYPE unknown_type;
DROP TYPE domain_using_unknown;
