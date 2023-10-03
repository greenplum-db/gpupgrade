-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

--------------------------------------------------------------------------------
-- Create and setup migratable objects
--------------------------------------------------------------------------------
SET search_path to tsquery_schema;

-- check tsquery data
SELECT * FROM tsquery_pt_table ORDER BY a;
SELECT * FROM tsquery_composite ORDER BY i;
SELECT * FROM tsquery_gist ORDER BY i;
SELECT * FROM tsquery_cluster_comment ORDER BY i;
SELECT * FROM tsquery_inherits ORDER BY a;

-- check tsquery relations
SELECT n.nspname, c.relname, a.attname
FROM pg_catalog.pg_class c
JOIN pg_catalog.pg_namespace n ON c.relnamespace = n.oid
JOIN pg_catalog.pg_attribute a ON c.oid = a.attrelid
WHERE c.relkind = 'r'
    AND NOT a.attisdropped
    AND a.atttypid = 'pg_catalog.tsquery'::pg_catalog.regtype
    AND n.nspname !~ '^pg_temp_'
    AND n.nspname !~ '^pg_toast_temp_'
    AND n.nspname NOT IN ('pg_catalog', 'information_schema')
    AND c.oid NOT IN (
        SELECT DISTINCT parchildrelid
        FROM pg_catalog.pg_partition_rule
    )
ORDER BY 1, 2, 3;

-- check indexes
SELECT c.relname AS index_name
FROM pg_index i
JOIN pg_class c ON i.indexrelid = c.oid
JOIN pg_class t ON i.indrelid = t.oid
WHERE t.relname LIKE 'tsquery%';

-- check comment
SELECT c.relname AS index_name, d.description AS index_comment
FROM pg_index i
JOIN pg_class c ON i.indexrelid = c.oid
LEFT JOIN pg_description d ON c.oid = d.objoid
WHERE c.relname = 'tsquery_cluster_comment_idx'
AND d.objsubid = 0;

-- check views
SELECT schemaname, viewname
FROM pg_views
WHERE schemaname NOT IN ('pg_catalog', 'information_schema', 'gp_toolkit')
AND schemaname = 'tsquery_schema'
ORDER BY 1, 2;

-- check view owners
SELECT schemaname, viewname, viewowner
FROM pg_views
WHERE schemaname NOT IN ('pg_catalog', 'information_schema', 'gp_toolkit')
AND schemaname = 'tsquery_schema'
AND viewowner = 'test_role1'
ORDER BY 1, 2, 3;

INSERT INTO tsquery_pt_table VALUES (1, 'b & c'::tsquery, 'b & c'::tsquery, 'b & c'::tsquery);
INSERT INTO tsquery_pt_table VALUES (2, 'e & f'::tsquery, 'e & f'::tsquery, 'e & f'::tsquery);
INSERT INTO tsquery_pt_table VALUES (3, 'x & y'::tsquery, 'x & y'::tsquery, 'x & y'::tsquery);

INSERT INTO tsquery_composite VALUES (1, 'b & c'::tsquery, 'b & c'::tsquery);
INSERT INTO tsquery_composite VALUES (2, 'e & f'::tsquery, 'e & f'::tsquery);
INSERT INTO tsquery_composite VALUES (3, 'x & y'::tsquery, 'x & y'::tsquery);

INSERT INTO tsquery_gist VALUES (1, 'b & c'::tsquery, 'b & c'::tsquery);
INSERT INTO tsquery_gist VALUES (2, 'e & f'::tsquery, 'e & f'::tsquery);
INSERT INTO tsquery_gist VALUES (3, 'x & y'::tsquery, 'x & y'::tsquery);

INSERT INTO tsquery_cluster_comment VALUES (1, 'b & c'::tsquery, 'b & c'::tsquery);
INSERT INTO tsquery_cluster_comment VALUES (2, 'e & f'::tsquery, 'e & f'::tsquery);
INSERT INTO tsquery_cluster_comment VALUES (3, 'x & y'::tsquery, 'x & y'::tsquery);

INSERT INTO tsquery_inherits VALUES (1, 'b & c'::tsquery, 'b & c'::tsquery, 'b & c'::tsquery, 'a & a'::tsquery);
INSERT INTO tsquery_inherits VALUES (2, 'e & f'::tsquery, 'e & f'::tsquery, 'e & f'::tsquery, 'b & b'::tsquery);
INSERT INTO tsquery_inherits VALUES (3, 'x & y'::tsquery, 'x & y'::tsquery, 'x & y'::tsquery, 'c & c'::tsquery);

-- check tsquery data
SELECT * FROM tsquery_pt_table ORDER BY a;
SELECT * FROM tsquery_composite ORDER BY i;
SELECT * FROM tsquery_gist ORDER BY i;
SELECT * FROM tsquery_cluster_comment ORDER BY i;
SELECT * FROM tsquery_inherits ORDER BY a;

