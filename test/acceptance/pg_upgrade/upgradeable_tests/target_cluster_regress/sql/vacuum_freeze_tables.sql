-- Copyright (c) 2017-2021 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

--------------------------------------------------------------------------------
-- Validate that the upgradeable objects are functional post-upgrade
--------------------------------------------------------------------------------

-- should be able to vacuum freeze the tables
VACUUM FREEZE vf_tbl_heap;
VACUUM FREEZE vf_tbl_ao;
VACUUM FREEZE vf_tbl_aoco;

-- should be able to create a new table without any warnings related to vacuum
CREATE TABLE upgraded_vf_tbl_heap (LIKE vf_tbl_heap);
INSERT INTO upgraded_vf_tbl_heap SELECT * FROM vf_tbl_heap;
VACUUM FREEZE upgraded_vf_tbl_heap;
SELECT * FROM upgraded_vf_tbl_heap;
