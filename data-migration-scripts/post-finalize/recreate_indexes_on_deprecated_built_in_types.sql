-- Copyright (c) 2017-2022 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- generates create index statement to re-create indexes on deprecated types.

SELECT pg_get_indexdef(xc.oid) || ';'
           ||
       CASE WHEN x.indisclustered
                THEN
                    chr(10) ||
                    $$ALTER TABLE $$ ||
        pg_catalog.quote_ident(n.nspname) || '.' || pg_catalog.quote_ident(c.relname) ||
        $$ CLUSTER ON $$ || pg_catalog.quote_ident(xc.relname) || ';'
    ELSE
    ''
END
||
CASE WHEN d.description IS NOT NULL
THEN
    chr(10) ||
    $$COMMENT ON INDEX $$ ||
    pg_catalog.quote_ident(n.nspname) || '.' || pg_catalog.quote_ident(xc.relname) ||
    $$ IS '$$ || d.description || $$';$$
ELSE
''
END
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
    AND n.nspname NOT IN ('pg_catalog', 'information_schema')
    AND c.oid NOT IN
        (SELECT DISTINCT parchildrelid
         FROM pg_catalog.pg_partition_rule);
