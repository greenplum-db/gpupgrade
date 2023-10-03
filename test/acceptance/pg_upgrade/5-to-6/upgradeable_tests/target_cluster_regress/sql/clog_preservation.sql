-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

--------------------------------------------------------------------------------
-- Validate that the upgradeable objects are functional post-upgrade
--------------------------------------------------------------------------------

-- We should be able to scan all tuples from this table without encountering
-- CLOG lookup failures, which proves that no CLOG was inadvertently truncated
-- during gpupgrade execute.
SELECT count(*) FROM foo;
