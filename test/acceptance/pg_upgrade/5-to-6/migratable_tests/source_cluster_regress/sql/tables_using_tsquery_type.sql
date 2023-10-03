-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

--------------------------------------------------------------------------------
-- Create and setup migratable objects
--------------------------------------------------------------------------------
CREATE SCHEMA tsquery_schema;
SET search_path to tsquery_schema;

-- partition table with columns of tsquery datatype
CREATE TABLE tsquery_pt_table (
    a int,
    b tsquery,
    c tsquery,
    d tsquery
) PARTITION BY RANGE (a) (
    START (1) END (4) EVERY (1)
);
INSERT INTO tsquery_pt_table VALUES (1, 'b & c'::tsquery, 'b & c'::tsquery, 'b & c'::tsquery);
INSERT INTO tsquery_pt_table VALUES (2, 'e & f'::tsquery, 'e & f'::tsquery, 'e & f'::tsquery);
INSERT INTO tsquery_pt_table VALUES (3, 'x & y'::tsquery, 'x & y'::tsquery, 'x & y'::tsquery);

-- composite index
CREATE TABLE tsquery_composite(i int, j tsquery, k tsquery);
CREATE INDEX tsquery_composite_idx ON tsquery_composite(j, k);
INSERT INTO tsquery_composite VALUES (1, 'b & c'::tsquery, 'b & c'::tsquery);
INSERT INTO tsquery_composite VALUES (2, 'e & f'::tsquery, 'e & f'::tsquery);
INSERT INTO tsquery_composite VALUES (3, 'x & y'::tsquery, 'x & y'::tsquery);

-- gist index
CREATE TABLE tsquery_gist(i int, j tsquery, k tsquery);
CREATE INDEX tsquery_gist_idx ON tsquery_gist using gist(j) ;
INSERT INTO tsquery_gist VALUES (1, 'b & c'::tsquery, 'b & c'::tsquery);
INSERT INTO tsquery_gist VALUES (2, 'e & f'::tsquery, 'e & f'::tsquery);
INSERT INTO tsquery_gist VALUES (3, 'x & y'::tsquery, 'x & y'::tsquery);

-- clustered index with comment
CREATE TABLE tsquery_cluster_comment(i int, j tsquery, k tsquery);
CREATE INDEX tsquery_cluster_comment_idx ON tsquery_cluster_comment(j);
ALTER TABLE tsquery_cluster_comment CLUSTER ON tsquery_cluster_comment_idx;
COMMENT ON INDEX tsquery_cluster_comment_idx IS 'hello world';
INSERT INTO tsquery_cluster_comment VALUES (1, 'b & c'::tsquery, 'b & c'::tsquery);
INSERT INTO tsquery_cluster_comment VALUES (2, 'e & f'::tsquery, 'e & f'::tsquery);
INSERT INTO tsquery_cluster_comment VALUES (3, 'x & y'::tsquery, 'x & y'::tsquery);

-- inherits with tsquery column
CREATE TABLE tsquery_inherits (e tsquery) INHERITS (tsquery_pt_table);
INSERT INTO tsquery_inherits VALUES (1, 'b & c'::tsquery, 'b & c'::tsquery, 'b & c'::tsquery, 'a & a'::tsquery);
INSERT INTO tsquery_inherits VALUES (2, 'e & f'::tsquery, 'e & f'::tsquery, 'e & f'::tsquery, 'b & b'::tsquery);
INSERT INTO tsquery_inherits VALUES (3, 'x & y'::tsquery, 'x & y'::tsquery, 'x & y'::tsquery, 'c & c'::tsquery);

-- extra tables for views that depend on tables using tsquery
CREATE TABLE tsquery_table1 (
    name     text,
    altitude tsquery
);
CREATE INDEX tsquery_table1_idx ON tsquery_table1(altitude);

CREATE TABLE tsquery_table2 (
    b tsquery
);

-- view dependency tests on deprecated tsquery
-- view on tsquery from a table
CREATE VIEW view_on_tsquery AS SELECT * FROM tsquery_table1;

-- view on tsquery from multiple tables
CREATE VIEW view_on_tsquery_mult_tables AS SELECT t1.name, t2.b FROM tsquery_table1 t1, tsquery_table2 t2;

-- view on tsquery from a table and a view
CREATE VIEW view_on_tsquery_table_view AS SELECT t1.name, v1.altitude FROM tsquery_table1 t1, view_on_tsquery v1;

-- view on tsquery from multiple views
CREATE VIEW view_on_tsquery_mult_views AS SELECT v1.name, v2.altitude FROM view_on_tsquery v1, view_on_tsquery_table_view v2;

-- view on tsquery from a table and multiple views
CREATE VIEW view_on_tsquery_table_mult_views AS SELECT t2.b, v1.name, v2.altitude FROM tsquery_table2 t2, view_on_tsquery v1, view_on_tsquery_table_view v2;

-- view on tsquery from a table to make sure that the creation order of the views does not affect drop order
CREATE VIEW view_on_tsquery_creation_order AS SELECT * FROM tsquery_table1;

-- view on tsquery from multiple tables and multiple views
CREATE VIEW view_on_tsquery_mult_tables_mult_views AS SELECT t1.name, t2.b, v1.altitude FROM tsquery_table1 t1, tsquery_table2 t2, view_on_tsquery v1, view_on_tsquery_mult_tables v2;
ALTER TABLE view_on_tsquery_mult_tables_mult_views OWNER TO migratable_objects_role;

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
AND viewowner = 'migratable_objects_role'
ORDER BY 1, 2, 3;
