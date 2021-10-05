-- Copyright (c) 2017-2021 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- This script assumes that the plpython language is already enabled
-- The generator should enable it if necessary on each database prior to this point

-- TODO: This implementation can be greatly simplified using recursive CTEs
-- They will be supported for 6X -> 7X upgrades

SET client_min_messages TO WARNING;

DROP SCHEMA IF EXISTS __gpupgrade_tmp;
CREATE SCHEMA __gpupgrade_tmp;

CREATE OR REPLACE FUNCTION  __gpupgrade_tmp.find_view_dependencies()
RETURNS VOID AS
$$
import plpy

# First find views that do not depend on other views (and directly on the table)

leaf_view = plpy.execute("""
SELECT schema, view
FROM (
SELECT DISTINCT nv.nspname AS schema, v.relname AS view
FROM pg_depend d
    JOIN pg_rewrite r ON r.oid = d.objid
    JOIN pg_class v ON v.oid = r.ev_class
    JOIN pg_catalog.pg_namespace nv ON v.relnamespace = nv.oid
    JOIN pg_catalog.pg_attribute a ON (d.refobjid = a.attrelid AND d.refobjsubid = a.attnum)
    JOIN pg_catalog.pg_class c ON c.oid = a.attrelid
    JOIN pg_catalog.pg_namespace nc ON c.relnamespace = nc.oid

WHERE
    v.relkind = 'v'
    AND d.classid = 'pg_rewrite'::regclass
    AND d.refclassid = 'pg_class'::regclass
    AND d.deptype = 'n'
    AND (a.atttypid = 'pg_catalog.tsquery'::pg_catalog.regtype OR a.atttypid = 'pg_catalog.name'::pg_catalog.regtype)
    AND c.relkind = 'r'
    AND NOT a.attisdropped
    AND nv.nspname NOT LIKE 'pg_temp_%'
    AND nv.nspname NOT LIKE 'pg_toast_temp_%'
    AND nv.nspname NOT IN ('pg_catalog',
    'information_schema')
    AND nc.nspname NOT LIKE 'pg_temp_%'
    AND nc.nspname NOT LIKE 'pg_toast_temp_%'
    AND nc.nspname NOT IN ('pg_catalog',
                        'information_schema')
) subq;
""")

checklist = {}
view_order = 1
for row in leaf_view:
    checklist[(row['schema'], row['view'])] = view_order

rows = plpy.execute("""
    SELECT
        nsp1.nspname AS depender_schema,
        depender,
        nsp2.nspname AS dependee_schema,
        dependee
    FROM
        pg_namespace AS nsp1,
        pg_namespace AS nsp2,
        (
            SELECT
                c.relname depender,
                c.relnamespace AS depender_nsp,
                c1.relname AS dependee,
                c1.relnamespace AS dependee_nsp
            FROM
                pg_rewrite AS rw,
                pg_depend AS d,
                pg_class AS c,
                pg_class AS c1
            WHERE
                rw.ev_class = c.oid AND
                rw.oid = d.objid AND
                d.classid = 'pg_rewrite'::regclass AND
                d.refclassid = 'pg_class'::regclass AND
                d.refobjid = c1.oid AND
                c1.relkind = 'v' AND
                c.relname <> c1.relname
            GROUP BY
                depender, depender_nsp, dependee, dependee_nsp
        ) t1
    WHERE
        t1.depender_nsp = nsp1.oid AND
        t1.dependee_nsp = nsp2.oid
        AND nsp1.nspname NOT LIKE 'pg_temp_%'
        AND nsp1.nspname NOT LIKE 'pg_toast_temp_%'
        AND nsp1.nspname NOT IN ('pg_catalog',
        'information_schema', 'gp_toolkit')
        AND nsp2.nspname NOT LIKE 'pg_temp_%'
        AND nsp2.nspname NOT LIKE 'pg_toast_temp_%'
        AND nsp2.nspname NOT IN ('pg_catalog',
                            'information_schema', 'gp_toolkit')
""")

view2view = {}
for row in rows:
    key = (row['depender_schema'], row['depender'])
    val = (row['dependee_schema'], row['dependee'])
    view2view[key]=val

while True:
    view_order += 1
    new_checklist = {}
    for depender, dependee in view2view.iteritems():
        if dependee in checklist and depender not in checklist:
            new_checklist[depender] = view_order
    if len(new_checklist) == 0:
        break
    else:
        checklist.update(new_checklist)

plpy.execute("DROP TABLE IF EXISTS  __gpupgrade_tmp.__temp_views_list")
plpy.execute("CREATE TABLE  __gpupgrade_tmp.__temp_views_list (full_view_name TEXT, view_order INTEGER)")
for v, view_order in checklist.items():
    sql = "INSERT INTO  __gpupgrade_tmp.__temp_views_list VALUES('{0}.{1}', {2})".format(v[0],v[1],view_order)
    plpy.execute(sql)
$$ LANGUAGE plpythonu;

SELECT  __gpupgrade_tmp.find_view_dependencies();

DROP FUNCTION  __gpupgrade_tmp.find_view_dependencies();
