-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

--------------------------------------------------------------------------------
-- Create and setup non-upgradeable objects
--------------------------------------------------------------------------------

-- The query that looks for these types had to be rewritten for 6 > 7 upgrade
-- because the recursive query looking for these types of relations contained a
-- self reference in a subquery. This specific type of query is disabled in 6x
-- so it was rewritten in plpgsql.

CREATE TYPE range_using_reg AS RANGE (
    subtype = regproc
);
CREATE DOMAIN domain_using_reg AS range_using_reg;
CREATE TABLE table_using_reg (
	col1 regconfig,
	col2 regdictionary,
	col3 regoper,
	col4 regoperator,
	col5 regproc,
	col6 regprocedure,
    col7 range_using_reg,
    col8 domain_using_reg
);

-- build custom types that depend on each other to test recursive query used to
-- find the tables that depend on reg types.
CREATE TYPE reg_type AS (
	t0 regproc
);
CREATE TYPE arr_reg_type1 AS (
	t1 reg_type[]
);
CREATE TYPE arr_reg_type2 AS (
	t2 arr_reg_type1[]
);
CREATE TYPE arr_reg_type3 AS (
	t3 arr_reg_type2[]
);
CREATE TABLE table_using_multiple_layers_of_reg_type (
    col1 reg_type,
    col2 arr_reg_type1,
    col3 arr_reg_type2,
    col4 arr_reg_type3
);

---------------------------------------------------------------------------------
--- Assert that pg_upgrade --check correctly detects the non-upgradeable objects
---------------------------------------------------------------------------------
!\retcode gpupgrade initialize --source-gphome="${GPHOME_SOURCE}" --target-gphome=${GPHOME_TARGET} --source-master-port=${PGPORT} --disk-free-ratio 0 --non-interactive;
! find $(ls -dt ~/gpAdminLogs/gpupgrade/pg_upgrade_*/ | head -1) -name "tables_using_reg.txt" -exec cat {} +;

---------------------------------------------------------------------------------
--- Workaround to unblock upgrade
---------------------------------------------------------------------------------
DROP TABLE table_using_multiple_layers_of_reg_type;
DROP TABLE table_using_reg;

DROP TYPE arr_reg_type3;
DROP TYPE arr_reg_type2;
DROP TYPE arr_reg_type1;
DROP TYPE reg_type;
DROP TYPE domain_using_reg;
DROP TYPE range_using_reg;
