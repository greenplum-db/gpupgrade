-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- This script assumes that the plpython language is already enabled
-- The generator should enable it if necessary on each database prior to this point

-- TODO: This implementation can be greatly simplified using recursive CTEs
-- They will be supported for 6X -> 7X upgrades

SET client_min_messages TO WARNING;

CREATE OR REPLACE FUNCTION  __gpupgrade_tmp_generator.find_view_dependencies()
RETURNS void AS
$$
import plpy

# Deprecated views are views whose tables contain a deprecated object.
# The sql joins on pg_rewrite to get view names because the pg_depend entries for
# views are against their internal definitions in pg_rewrite rather than their
# relational definition in pg_class.
deprecated_views_result = plpy.execute("""
SELECT
    view_schema,
    view_name,
    view_owner
FROM (
    SELECT DISTINCT
        nv.nspname AS view_schema,
        v.relname AS view_name,
        pg_catalog.pg_get_userbyid(v.relowner) AS view_owner
    FROM
        pg_depend d
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
        AND a.atttypid = 'pg_catalog.tsquery'::pg_catalog.regtype
        AND c.relkind = 'r'
        AND NOT a.attisdropped
        AND nv.nspname NOT LIKE 'pg_temp_%'
        AND nv.nspname NOT LIKE 'pg_toast_temp_%'
        AND nv.nspname NOT IN ('pg_catalog', 'information_schema')
        AND nc.nspname NOT LIKE 'pg_temp_%'
        AND nc.nspname NOT LIKE 'pg_toast_temp_%'
        AND nc.nspname NOT IN ('pg_catalog', 'information_schema')
    ) subq;
""")

# Get views that depend on other views
dependent_views_result = plpy.execute("""
SELECT
    nsp1.nspname AS view_schema,
    view_name,
    view_owner,
    nsp2.nspname AS dependent_view_schema,
    dependent_view_name,
    dependent_view_owner
FROM
    pg_namespace AS nsp1,
    pg_namespace AS nsp2,
    (
        SELECT
            c.relname view_name,
            c.relnamespace AS view_nsp,
            pg_catalog.pg_get_userbyid(c.relowner) AS view_owner,
            c1.relname AS dependent_view_name,
            c1.relnamespace AS dependent_view_nsp,
            pg_catalog.pg_get_userbyid(c1.relowner) AS dependent_view_owner
        FROM
            pg_rewrite AS rw,
            pg_depend AS d,
            pg_class AS c,
            pg_class AS c1
        WHERE
            rw.ev_class = c.oid
            AND rw.oid = d.objid
            AND d.classid = 'pg_rewrite'::regclass
            AND d.refclassid = 'pg_class'::regclass
            AND d.refobjid = c1.oid
            AND c1.relkind = 'v'
            AND c.relname <> c1.relname
        GROUP BY
            view_name, view_nsp, view_owner, dependent_view_name, dependent_view_nsp, dependent_view_owner
    ) t1
WHERE
    t1.view_nsp = nsp1.oid
    AND t1.dependent_view_nsp = nsp2.oid
    AND nsp1.nspname NOT LIKE 'pg_temp_%'
    AND nsp1.nspname NOT LIKE 'pg_toast_temp_%'
    AND nsp1.nspname NOT IN ('pg_catalog', 'information_schema', 'gp_toolkit')
    AND nsp2.nspname NOT LIKE 'pg_temp_%'
    AND nsp2.nspname NOT LIKE 'pg_toast_temp_%'
    AND nsp2.nspname NOT IN ('pg_catalog', 'information_schema', 'gp_toolkit')
""")

class View:
    def __init__(self, schema, name, owner):
        self.schema = schema
        self.name = name
        self.owner = owner
        self.visited = False

    def __eq__(self, other):
        if isinstance(other, View):
            return self.schema == other.schema and self.name == other.name and self.owner == other.owner
        return False

    def __hash__(self):
        return hash((self.schema, self.name, self.owner))

deprecated_views = []
dependent_view_to_views = {}  # a dependent view is one that references one or more view
deprecated_view_to_dependent_views = {}
for row in deprecated_views_result:
    deprecated_view = View(row['view_schema'], row['view_name'], row['view_owner'])
    deprecated_views.append(deprecated_view)
    dependent_view_to_views[deprecated_view] = []
    deprecated_view_to_dependent_views[deprecated_view] = []

# Build reversed dependency map. This is done because it will be easier to
# discover deprecated views starting from known deprecated views.
for row in dependent_views_result:
    view = View(row['view_schema'], row['view_name'], row['view_owner'])
    dependent_view = View(row['dependent_view_schema'], row['dependent_view_name'], row['dependent_view_owner'])
    dependent_view_to_views.setdefault(view, []) # Add view to the dependency map, but no value is needed since there are no other views that depend on it.
    dependent_view_to_views.setdefault(dependent_view, []).append(view)

def create_dependency_graph(graph, node, dependency_graph):
    if node.visited:
        return

    node.visited = True
    for neighbor in graph[node]:
        dependency_graph.setdefault(neighbor, []).append(node)
        create_dependency_graph(graph, neighbor, dependency_graph)

deprecated_view_to_dependencies = {}
for view in deprecated_views:
    create_dependency_graph(dependent_view_to_views, view, deprecated_view_to_dependencies)

# Add deprecated views and their dependencies to deprecated_view_to_dependent_views
for deprecated_view, dependencies in deprecated_view_to_dependencies.items():
    deprecated_view_to_dependent_views.setdefault(deprecated_view, []).extend(dependencies)

def sort_nodes(graph, node):
    if node.visited:
        return []

    node.visited = True
    undiscovered_neighbors = []
    for neighbor in graph[node]:
        undiscovered_neighbors += sort_nodes(graph, neighbor)

    undiscovered_neighbors.append(node)
    return undiscovered_neighbors

def topological_sort(graph):
    # reset graph
    for node, nodes in graph.items():
        node.visited = False
        for neighbor in nodes:
            neighbor.visited = False

    sorted_nodes = []
    for node in graph:
        sorted_nodes += sort_nodes(graph, node)

    sorted_nodes.reverse()
    return sorted_nodes

# Sort deprecated views such that DROP VIEW statements will be generated in the
# correct order since some views dependent on others.
sorted_deprecated_views = topological_sort(deprecated_view_to_dependent_views)

plpy.execute("DROP TABLE IF EXISTS  __gpupgrade_tmp_generator.__temp_views_list")
plpy.execute("CREATE TABLE  __gpupgrade_tmp_generator.__temp_views_list (full_view_name TEXT, view_owner TEXT, view_order INTEGER)")
for index, view in enumerate(sorted_deprecated_views):
    sql = "INSERT INTO  __gpupgrade_tmp_generator.__temp_views_list VALUES('{0}.{1}', '{2}', {3})".format(view.schema, view.name, view.owner, index)
    plpy.execute(sql)
$$ LANGUAGE plpythonu;

SELECT __gpupgrade_tmp_generator.find_view_dependencies();

DROP FUNCTION __gpupgrade_tmp_generator.find_view_dependencies();
