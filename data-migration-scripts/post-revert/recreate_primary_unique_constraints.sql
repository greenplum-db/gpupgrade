-- Copyright (c) 2017-2021 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- Generate a script to to recreate unique/primary key constraints from
-- non-partitioned tables. Exclude all the root and child partitions as we
-- cannot drop unique/primary constraints on the child partition tables

-- cte to get oids of all tables that are not partition tables
WITH CTE as
(
    SELECT
        oid,
        *
    FROM
        pg_class
    WHERE
        oid NOT IN
        (
            SELECT DISTINCT
                parrelid
            FROM
                pg_partition
            UNION ALL
            SELECT DISTINCT
                parchildrelid
            FROM
                pg_partition_rule
        )
 )
SELECT
    $$ALTER TABLE $$ || pg_catalog.quote_ident(n.nspname) || $$.$$ ||
    pg_catalog.quote_ident(cc.relname) ||
    $$ ADD CONSTRAINT $$ || pg_catalog.quote_ident(conname) || $$ $$ ||
    pg_catalog.pg_get_constraintdef(con.oid, false)  || $$;$$
FROM
    pg_constraint con
        JOIN
    pg_depend dep
    ON (refclassid, classid, objsubid) =
       (
        'pg_constraint'::regclass,
        'pg_class'::regclass,
        0
           )
        AND refobjid = con.oid
        AND deptype = 'i'
        AND contype IN
            (
             'u',
             'p'
                )
        JOIN
    CTE c
    ON objid = c.oid
        AND relkind = 'i'
        JOIN
    CTE cc
    ON cc.oid = con.conrelid
        JOIN
    pg_namespace n
    ON (n.oid = cc.relnamespace)
WHERE
    conname <> c.relname ;
