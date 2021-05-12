-- Copyright (c) 2017-2021 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

--------------------------------------------------------------------------------
-- Validate that the upgradeable objects have been disabled during the upgrade
--------------------------------------------------------------------------------

-- bpchar_pattern_ops index should be marked invalid on master and segments
SELECT DISTINCT indisvalid FROM pg_index WHERE indexrelid = 'bpchar_idx'::regclass;
SELECT DISTINCT indisvalid FROM gp_dist_random('pg_index') WHERE indexrelid = 'bpchar_idx'::regclass;

-- bitmap index should be marked invalid on master and segments
SELECT DISTINCT indisvalid FROM pg_index WHERE indexrelid = 'bitmap_idx'::regclass;
SELECT DISTINCT indisvalid FROM gp_dist_random('pg_index') WHERE indexrelid = 'bitmap_idx'::regclass;

--------------------------------------------------------------------------------
-- Post-upgrade resolution to enable the upgradable objects
--------------------------------------------------------------------------------
REINDEX TABLE tbl_with_bpchar_pattern_ops_index;
REINDEX TABLE tbl_with_bitmap_index;

--------------------------------------------------------------------------------
-- Validate that the upgradeable objects are functional post-upgrade
--------------------------------------------------------------------------------

-- bpchar_pattern_ops index scan can now be be used
SET ENABLE_SEQSCAN=OFF;
SET ENABLE_INDEXSCAN=OFF;
EXPLAIN (COSTS OFF) SELECT * FROM tbl_with_bpchar_pattern_ops_index WHERE lower(b)::bpchar LIKE '1';
SELECT * FROM tbl_with_bpchar_pattern_ops_index WHERE lower(b)::bpchar LIKE '1';

-- bitmap index scan can now be used
EXPLAIN (COSTS OFF) SELECT * FROM tbl_with_bitmap_index WHERE b = '1';
SELECT * FROM tbl_with_bitmap_index WHERE b = '1';
