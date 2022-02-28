-- Copyright (c) 2017-2021 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

SELECT $$CREATE VIEW $$|| full_view_name || $$ AS $$ ||
    pg_catalog.pg_get_viewdef(full_view_name::regclass::oid, false) || $$;$$
FROM __gpupgrade_tmp_generator.__temp_views_list ORDER BY view_order;
