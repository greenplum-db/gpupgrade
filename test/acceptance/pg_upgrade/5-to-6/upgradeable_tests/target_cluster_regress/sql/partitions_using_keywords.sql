-- Copyright (c) 2017-2022 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

--------------------------------------------------------------------------------
-- Validate that the upgradeable objects are functional post-upgrade
--------------------------------------------------------------------------------

SELECT pg_get_partition_def('t_quote_test'::regclass);
