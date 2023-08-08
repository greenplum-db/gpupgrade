-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

--------------------------------------------------------------------------------
-- Create and setup migratable objects
--------------------------------------------------------------------------------

CREATE TABLE preserve_view_owner_table(i int);
CREATE VIEW preserve_view_owner_view AS SELECT i FROM preserve_view_owner_table;
ALTER TABLE preserve_view_owner_view OWNER TO test_role1;

SELECT schemaname, viewname, viewowner
FROM pg_views
WHERE schemaname NOT IN ('pg_catalog', 'information_schema', 'gp_toolkit')
ORDER BY 1,2,3;
