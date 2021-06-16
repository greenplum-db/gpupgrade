-- Copyright (c) 2017-2021 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- We have tables in the catalog that contain deprecated columns but aren't
-- deprecated themselves. Views that reference such columns error out during
-- schema restore, rendering them non-upgradeable. They must be dropped before
-- running an upgrade.

--------------------------------------------------------------------------------
-- Create and setup non-upgradeable objects
--------------------------------------------------------------------------------

-- Create views containing references to deprecated column replication_port in
-- various portions of a potential view query tree (such as subquery, join, CTE
-- etc) to ensure that check_node_deprecated_columns_walker correctly flags these
-- as non-upgradeable. Note that this is not an exhaustive list covering all
-- possible expression types.
-- GPDB5: gp_segment_configuration contains deprecated column replication_port
CREATE VIEW dep_col_tlist AS SELECT replication_port FROM gp_segment_configuration;
CREATE VIEW dep_col_qual AS SELECT 1 FROM gp_segment_configuration where replication_port > 8000;
CREATE VIEW dep_col_group_by AS SELECT count(*) from gp_segment_configuration GROUP BY replication_port;
CREATE VIEW dep_col_order_by AS SELECT dbid from gp_segment_configuration ORDER BY replication_port;
CREATE VIEW dep_col_cte AS (WITH c AS (SELECT replication_port FROM gp_segment_configuration) SELECT * FROM c);
CREATE VIEW dep_col_subquery AS SELECT 1 FROM (SELECT replication_port FROM gp_segment_configuration) sub;
CREATE VIEW dep_col_sublink AS SELECT 1 FROM gp_segment_configuration WHERE 8000 > ANY (SELECT replication_port FROM gp_segment_configuration);
CREATE VIEW dep_col_join AS SELECT 1 FROM gp_segment_configuration s1, (SELECT replication_port FROM gp_segment_configuration) s2;

-- Even if a column is not explicitly referenced, due to the way '*' is expanded
-- and stored, the following is deprecated (See view definition output below).
CREATE VIEW dep_col_tlist_star_expand AS SELECT * FROM gp_segment_configuration;
SELECT pg_get_viewdef('dep_col_tlist_star_expand'::regclass);

-- We have special logic to deal with joins containing the JOIN clause (as
-- opposed to to the traditional syntax). Joins with the JOIN clause result in
-- the construction of joinaliasvars, which we take special care not to recurse
-- into in check_node_deprecated_columns_walker.
CREATE VIEW dep_col_join_on AS SELECT 1 FROM gp_segment_configuration s1 JOIN gp_segment_configuration s2 ON s1.replication_port=s2.replication_port;
CREATE VIEW dep_col_join_using AS SELECT 1 FROM gp_segment_configuration s1 JOIN gp_segment_configuration s2 USING (replication_port);
CREATE VIEW dep_col_natural_join AS SELECT 1 FROM gp_segment_configuration s1 NATURAL JOIN (select replication_port from generate_series(15432, 15435)replication_port) ports;

-- Also test on a view containing a correlated subquery to validate the logic
-- used to look up the range table corresponding to an outer Var.
CREATE VIEW dep_col_correlated_subquery AS (SELECT dbid FROM gp_segment_configuration g1
    WHERE dbid = (SELECT dbid FROM gp_segment_configuration g2 WHERE g1.replication_port < g2.replication_port));

-- Create a view containing a reference to a deprecated column that is in the
-- gp_toolkit schema. We use a slightly different detection mechanism for such
-- tables (they don't have static Oids, so we have to perform a dynamic Oid
-- lookup)
CREATE VIEW dep_col_dynamic_oid AS SELECT proposed_concurrency FROM gp_toolkit.gp_resgroup_config;

--------------------------------------------------------------------------------
-- Assert that pg_upgrade --check correctly detects the non-upgradeable objects
--------------------------------------------------------------------------------
!\retcode gpupgrade initialize --source-gphome="${GPHOME_SOURCE}" --target-gphome=${GPHOME_TARGET} --source-master-port=${PGPORT} --disk-free-ratio 0 --automatic;
! cat ${GPUPGRADE_HOME}/pg_upgrade/seg-1/view_deprecated_columns.txt | LC_ALL=C sort -b;

--------------------------------------------------------------------------------
-- Workaround to unblock upgrade
--------------------------------------------------------------------------------
DROP VIEW dep_col_tlist;
DROP VIEW dep_col_qual;
DROP VIEW dep_col_group_by;
DROP VIEW dep_col_order_by;
DROP VIEW dep_col_cte;
DROP VIEW dep_col_subquery;
DROP VIEW dep_col_sublink;
DROP VIEW dep_col_join;
DROP VIEW dep_col_join_on;
DROP VIEW dep_col_join_using;
DROP VIEW dep_col_natural_join;

DROP VIEW dep_col_correlated_subquery;

DROP VIEW dep_col_dynamic_oid;
