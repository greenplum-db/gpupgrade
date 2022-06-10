-- Copyright (c) 2017-2022 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

SELECT
    $$CREATE VIEW $$ || full_view_name || $$ AS $$ || pg_catalog.pg_get_viewdef(full_view_name::regclass::oid, false) || $$;$$ || E'\n'||
    $$ALTER TABLE $$ || full_view_name || $$ OWNER TO $$ || view_owner || $$;$$
FROM __gpupgrade_tmp_generator.__temp_views_list ORDER BY view_order;
