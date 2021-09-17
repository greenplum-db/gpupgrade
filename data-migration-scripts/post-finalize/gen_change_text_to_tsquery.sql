-- Copyright (c) 2017-2021 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- generates alter statement to modify text datatype to tsquery datatype
SELECT $$ALTER TABLE $$ || pg_catalog.quote_ident(n.nspname) || '.' || pg_catalog.quote_ident(c.relname) ||
       $$ ALTER COLUMN $$ || pg_catalog.quote_ident(a.attname) ||
       $$ TYPE TSQUERY USING $$ || pg_catalog.quote_ident(a.attname) || $$::tsquery;$$
FROM pg_catalog.pg_class c,
     pg_catalog.pg_namespace n,
     pg_catalog.pg_attribute a
WHERE c.relkind = 'r'
  AND c.oid = a.attrelid
  AND NOT a.attisdropped
  AND a.atttypid = 'pg_catalog.tsquery'::pg_catalog.regtype
  AND c.relnamespace = n.oid
  AND n.nspname NOT LIKE 'pg_temp_%'
  AND n.nspname NOT LIKE 'pg_toast_temp_%'
  AND n.nspname NOT IN ('pg_catalog',
                        'information_schema')
  AND c.oid NOT IN
      (SELECT DISTINCT parchildrelid
       FROM pg_catalog.pg_partition_rule)
  AND a.attname NOT IN (
        SELECT a2.attname
        FROM
            pg_inherits AS i
            JOIN
                pg_attribute AS a2
                ON i.inhparent = a2.attrelid
        WHERE
            i.inhrelid = a.attrelid
            AND a.attname = a2.attname
    )
    AND c.oid NOT IN
    (
        SELECT DISTINCT d.refobjid
        FROM
            pg_depend d
            JOIN pg_rewrite r ON r.oid = d.objid
            JOIN pg_class v ON v.oid = r.ev_class
        WHERE
            relkind = 'v'
            AND d.classid = 'pg_rewrite'::regclass
            AND d.refclassid = 'pg_class'::regclass
            AND d.deptype = 'n'
    );
