-- Copyright (c) 2017-2021 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- generates create index statement to re-create indexes on deprecated types.

SET client_min_messages TO WARNING;

DROP VIEW IF EXISTS __gpupgrade_temp_view2;
DROP VIEW IF EXISTS __gpupgrade_temp_view;

CREATE VIEW __gpupgrade_temp_view AS
SELECT pg_get_indexdef(xc.oid) || ';' AS index,
        CASE WHEN x.indisclustered THEN
            $$ALTER TABLE $$ ||
            pg_catalog.quote_ident(n.nspname) || '.' || pg_catalog.quote_ident(c.relname) ||
            $$ CLUSTER ON $$ || pg_catalog.quote_ident(xc.relname) || ';'
        ELSE NULL
        END AS comment,
        CASE WHEN d.description IS NOT NULL THEN
            $$COMMENT ON INDEX $$ ||
                pg_catalog.quote_ident(n.nspname) || '.' || pg_catalog.quote_ident(xc.relname) ||
                $$ IS '$$ || d.description || $$';$$
        ELSE NULL
        END AS cluster
FROM
    pg_catalog.pg_class c
    JOIN pg_catalog.pg_namespace n ON c.relnamespace = n.oid
    JOIN pg_index x ON c.oid = x.indrelid
    JOIN pg_class xc ON x.indexrelid = xc.oid
    LEFT JOIN pg_description d ON xc.oid = d.objoid
WHERE
    EXISTS (
            SELECT 1 FROM pg_catalog.pg_attribute
            WHERE attrelid = c.oid
              AND attnum = ANY(x.indkey)
              AND (atttypid = 'pg_catalog.tsquery'::pg_catalog.regtype OR
                   atttypid = 'pg_catalog.name'::pg_catalog.regtype)
              AND NOT attisdropped
        )
    AND c.relkind = 'r'
    AND xc.relkind = 'i'
    AND n.nspname NOT LIKE 'pg_temp_%'
    AND n.nspname NOT LIKE 'pg_toast_temp_%'
    AND n.nspname NOT IN ('pg_catalog',
                        'information_schema')
    AND c.oid NOT IN
        (SELECT DISTINCT parchildrelid
         FROM pg_catalog.pg_partition_rule);

CREATE VIEW __gpupgrade_temp_view2 AS
SELECT 1 AS i, index AS command FROM __gpupgrade_temp_view
UNION
SELECT 2 AS i, comment AS command FROM __gpupgrade_temp_view WHERE comment IS NOT NULL
UNION
SELECT 3 AS i, cluster AS command FROM __gpupgrade_temp_view WHERE cluster IS NOT NULL;

SELECT command FROM __gpupgrade_temp_view2 ORDER BY i;

DROP VIEW IF EXISTS __gpupgrade_temp_view2;
DROP VIEW IF EXISTS __gpupgrade_temp_view;
