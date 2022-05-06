-- Copyright (c) 2017-2022 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

SELECT 'DROP VIEW '|| full_view_name || ';'
FROM  __gpupgrade_tmp_generator.__temp_views_list ORDER BY view_order DESC;
