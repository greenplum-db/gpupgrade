-- Copyright (c) 2017-2021 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

--------------------------------------------------------------------------------
-- Validate that the upgradeable objects are functional post-upgrade
--------------------------------------------------------------------------------
SELECT someimmutablepythonfunction(0) as f;

SELECT someimmutablepsqlfunction(0) as f;
