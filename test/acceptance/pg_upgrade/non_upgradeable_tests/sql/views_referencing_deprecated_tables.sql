-- Copyright (c) 2017-2021 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- GPDB5: Certain tables are deprecated with respect to a major version
-- upgrade from 5. Views that reference these tables error out during schema
-- restore, rendering them non-upgradeable. They must be dropped before running
-- an upgrade.

--------------------------------------------------------------------------------
-- Create and setup non-upgradeable objects
--------------------------------------------------------------------------------

-- Create views containing references to deprecated table pg_filespace_entry in
-- various portions of a potential view query tree (such as subquery, join and
-- CTE) to ensure that check_node_deprecated_tables_walker correctly flags these
-- as non-upgradeable.
CREATE VIEW dep_rel_view AS SELECT * from pg_filespace_entry;
CREATE VIEW dep_rel_view_subquery AS (SELECT * from (select * from pg_filespace_entry)sub);
CREATE VIEW dep_rel_view_join AS SELECT * from pg_filespace_entry, pg_database;
CREATE VIEW dep_rel_view_cte AS (WITH dep_rel_cte AS (SELECT * FROM pg_filespace_entry) SELECT * FROM dep_rel_cte);
CREATE VIEW dep_rel_sublink AS SELECT dbid FROM gp_segment_configuration WHERE 0 < ALL (SELECT fsedbid FROM pg_filespace_entry);

-- Create a view containing a reference to a deprecated table that is in the
-- gp_toolkit schema. We use a slightly different detection mechanism for such
-- tables (they don't have static Oids, so we have to perform a dynamic Oid
-- lookup)
CREATE VIEW dep_rel_dynamic_oid AS SELECT * FROM gp_toolkit.__gp_localid;

--------------------------------------------------------------------------------
-- Assert that pg_upgrade --check correctly detects the non-upgradeable objects
--------------------------------------------------------------------------------
!\retcode gpupgrade initialize --source-gphome="${GPHOME_SOURCE}" --target-gphome=${GPHOME_TARGET} --source-master-port=${PGPORT} --disk-free-ratio 0 --automatic;
! cat ${GPUPGRADE_HOME}/pg_upgrade/seg-1/view_deprecated_tables.txt | LC_ALL=C sort -b;

--------------------------------------------------------------------------------
-- Workaround to unblock upgrade
--------------------------------------------------------------------------------
DROP VIEW dep_rel_view;
DROP VIEW dep_rel_view_subquery;
DROP VIEW dep_rel_view_join;
DROP VIEW dep_rel_view_cte;
DROP VIEW dep_rel_sublink;

DROP VIEW dep_rel_dynamic_oid;
