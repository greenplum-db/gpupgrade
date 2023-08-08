-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

--------------------------------------------------------------------------------
-- Create and setup migratable objects
--------------------------------------------------------------------------------

SELECT schemaname, viewname, viewowner
FROM pg_views
WHERE schemaname NOT IN ('pg_catalog', 'information_schema', 'gp_toolkit')
ORDER BY 1,2,3;
