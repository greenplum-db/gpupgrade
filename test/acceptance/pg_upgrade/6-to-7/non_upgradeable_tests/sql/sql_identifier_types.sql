-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

--------------------------------------------------------------------------------
-- Create and setup non-upgradeable objects
--------------------------------------------------------------------------------

-- The query that looks for these types had to be rewritten for 6 > 7 upgrade
-- because the recursive query looking for these types of relations contained a
-- self reference in a subquery. This specific type of query is disabled in 6x
-- so it was rewritten in plpgsql.

CREATE TYPE range_using_sql_identifier AS RANGE (
    subtype = information_schema.sql_identifier
);
CREATE DOMAIN domain_using_sql_identifier AS range_using_sql_identifier;
CREATE TABLE table_using_sql_identifier (
	col1 information_schema.sql_identifier,
	col2 range_using_sql_identifier,
	col3 domain_using_sql_identifier
);

-- build custom types that depend on each other to test recursive query used to
-- find the tables that depend on information_schema.sql_identifier types.
CREATE TYPE sql_identifier_type AS (
	t0 information_schema.sql_identifier
);
CREATE TYPE arr_sql_identifier_type1 AS (
	t1 sql_identifier_type[]
);
CREATE TYPE arr_sql_identifier_type2 AS (
	t2 arr_sql_identifier_type1[]
);
CREATE TYPE arr_sql_identifier_type3 AS (
	t3 arr_sql_identifier_type2[]
);
CREATE TABLE table_using_multiple_layers_of_sql_identifier_type (
    col1 sql_identifier_type,
    col2 arr_sql_identifier_type1,
    col3 arr_sql_identifier_type2,
    col4 arr_sql_identifier_type3
);

---------------------------------------------------------------------------------
--- Assert that pg_upgrade --check correctly detects the non-upgradeable objects
---------------------------------------------------------------------------------
!\retcode gpupgrade initialize --source-gphome="${GPHOME_SOURCE}" --target-gphome=${GPHOME_TARGET} --source-master-port=${PGPORT} --disk-free-ratio 0 --non-interactive;
! cat ~/gpAdminLogs/gpupgrade/pg_upgrade/p-1/tables_using_sql_identifier.txt;

---------------------------------------------------------------------------------
--- Workaround to unblock upgrade
---------------------------------------------------------------------------------
DROP TABLE table_using_multiple_layers_of_sql_identifier_type;
DROP TABLE table_using_sql_identifier;

DROP TYPE arr_sql_identifier_type3;
DROP TYPE arr_sql_identifier_type2;
DROP TYPE arr_sql_identifier_type1;
DROP TYPE sql_identifier_type;
DROP TYPE domain_using_sql_identifier;
DROP TYPE range_using_sql_identifier;
