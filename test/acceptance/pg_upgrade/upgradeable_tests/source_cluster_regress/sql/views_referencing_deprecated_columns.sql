-- Copyright (c) 2017-2022 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- We have tables in the catalog that contain deprecated columns but aren't
-- deprecated themselves. Views that reference such columns error out during
-- schema restore, rendering them non-upgradeable. However, views that contain
-- joins of these tables and don't explicitly reference such columns are
-- upgradeable.

--------------------------------------------------------------------------------
-- Create and setup upgradeable objects
--------------------------------------------------------------------------------

-- Create views that don't explicitly reference a deprecated column.
-- GPDB5: gp_segment_configuration contains deprecated column replication_port

CREATE VIEW non_dep_col_simple_join AS SELECT 1 FROM gp_segment_configuration s1, gp_segment_configuration s2;
-- We have special logic to deal with joins containing the JOIN clause (as
-- opposed to to the traditional syntax). Joins with the JOIN clause result in
-- the construction of joinaliasvars, which we take special care not to recurse
-- into in check_node_deprecated_columns_walker.
CREATE VIEW non_dep_col_join_on AS SELECT 1 FROM gp_segment_configuration s1 JOIN gp_segment_configuration s2 ON s1.dbid=s2.dbid;
CREATE VIEW non_dep_col_join_using AS SELECT 1 FROM gp_segment_configuration s1 JOIN gp_segment_configuration s2 USING (dbid);
CREATE VIEW non_dep_col_natural_join AS SELECT 1 FROM gp_segment_configuration s1 NATURAL JOIN (select dbid from generate_series(1, 8)dbid) dbids;

SELECT pg_get_viewdef('non_dep_col_simple_join'::regclass);
SELECT pg_get_viewdef('non_dep_col_join_on'::regclass);
SELECT pg_get_viewdef('non_dep_col_join_using'::regclass);
SELECT pg_get_viewdef('non_dep_col_natural_join'::regclass);
