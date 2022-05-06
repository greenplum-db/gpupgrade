-- Copyright (c) 2017-2022 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- Test to ensure that views containing lag/lead functions with their first
-- argument of type bigint are upgraded successfully.

--------------------------------------------------------------------------------
-- Create and setup upgradeable objects
--------------------------------------------------------------------------------
CREATE TABLE lead_lag_tbl (a int, b text) DISTRIBUTED RANDOMLY;
INSERT INTO lead_lag_tbl SELECT 1, 'a';

CREATE VIEW lag_view AS SELECT lag(1::bigint) OVER (ORDER BY b) as lag FROM lead_lag_tbl;
CREATE VIEW lead_view AS SELECT lead(1::bigint) OVER (ORDER BY b) as lag FROM lead_lag_tbl;
