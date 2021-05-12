-- Copyright (c) 2017-2021 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- Test to ensure that tables with columns having non-default statistic targets
-- (modified with ALTER TABLE .. SET STATISTICS) can be upgraded.

--------------------------------------------------------------------------------
-- Create and setup upgradeable objects
--------------------------------------------------------------------------------
CREATE TABLE explicitly_set_statistic_table (
    col1 integer NOT NULL
);
ALTER TABLE ONLY explicitly_set_statistic_table ALTER COLUMN col1 SET STATISTICS 10;
INSERT INTO explicitly_set_statistic_table SELECT i FROM generate_series(1,10)i;
