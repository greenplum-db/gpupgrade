-- Copyright (c) 2017-2021 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

--------------------------------------------------------------------------------
-- Validate that the upgradeable objects are functional post-upgrade
--------------------------------------------------------------------------------

SELECT pg_get_viewdef('non_dep_col_simple_join'::regclass);
SELECT pg_get_viewdef('non_dep_col_join_on'::regclass);
SELECT pg_get_viewdef('non_dep_col_join_using'::regclass);
SELECT pg_get_viewdef('non_dep_col_natural_join'::regclass);
