-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

--------------------------------------------------------------------------------
-- Validate that the indexes still work after upgrade
--------------------------------------------------------------------------------

-- start_matchsubs
-- m/ERROR:  Unexpected internal error.*/
-- s/ERROR:  Unexpected internal error.*/ERROR:  Unexpected internal error/
-- m/DETAIL:  FailedAssertion.*/
-- s/DETAIL:  FailedAssertion.*/DETAIL:  FailedAssertion/
-- end_matchsubs

-- Show what the indexes are after upgrade
SELECT c.relname, a.amname, i.indisvalid FROM pg_class c JOIN pg_am a ON c.relam = a.oid JOIN pg_index i ON i.indexrelid = c.oid WHERE c.relname SIMILAR TO '(ao|aoco)_with_(btree|bitmap|gist)_idx';

-- Show that the indexes are not usable after upgrade due to expected
-- 5-to-6 AO index issues. For the AO bitmap index, the index is
-- invalid (as shown in the above catalog query) so it'll always use
-- sequential scan on the table which is why the query works.
SET enable_seqscan = off;
SET gp_debug_linger = 0;
EXPLAIN (COSTS off) SELECT * FROM ao_with_btree WHERE a > 8;
EXPLAIN (COSTS off) SELECT * FROM ao_with_bitmap WHERE a = 1;
EXPLAIN (COSTS off) SELECT * FROM ao_with_gist WHERE a @@ to_tsquery('footext1');
EXPLAIN (COSTS off) SELECT * FROM aoco_with_btree WHERE a > 8;
EXPLAIN (COSTS off) SELECT * FROM aoco_with_bitmap WHERE a = 1;
EXPLAIN (COSTS off) SELECT * FROM aoco_with_gist WHERE a @@ to_tsquery('footext1');
SELECT * FROM ao_with_btree WHERE a > 8;
SELECT * FROM ao_with_bitmap WHERE a = 1;
SELECT * FROM ao_with_gist WHERE a @@ to_tsquery('footext1');
SELECT * FROM aoco_with_btree WHERE a > 8;
SELECT * FROM aoco_with_bitmap WHERE a = 1;
SELECT * FROM aoco_with_gist WHERE a @@ to_tsquery('footext1');

-- Provided REINDEX workaround should fix all the AO indexes
REINDEX INDEX ao_with_btree_idx;
REINDEX INDEX ao_with_bitmap_idx;
REINDEX INDEX ao_with_gist_idx;
REINDEX INDEX aoco_with_btree_idx;
REINDEX INDEX aoco_with_bitmap_idx;
REINDEX INDEX aoco_with_gist_idx;

EXPLAIN (COSTS off) SELECT * FROM ao_with_btree WHERE a > 8;
EXPLAIN (COSTS off) SELECT * FROM ao_with_bitmap WHERE a = 1;
EXPLAIN (COSTS off) SELECT * FROM ao_with_gist WHERE a @@ to_tsquery('footext1');
EXPLAIN (COSTS off) SELECT * FROM aoco_with_btree WHERE a > 8;
EXPLAIN (COSTS off) SELECT * FROM aoco_with_bitmap WHERE a = 1;
EXPLAIN (COSTS off) SELECT * FROM aoco_with_gist WHERE a @@ to_tsquery('footext1');
SELECT * FROM ao_with_btree WHERE a > 8;
SELECT * FROM ao_with_bitmap WHERE a = 1;
SELECT * FROM ao_with_gist WHERE a @@ to_tsquery('footext1');
SELECT * FROM aoco_with_btree WHERE a > 8;
SELECT * FROM aoco_with_bitmap WHERE a = 1;
SELECT * FROM aoco_with_gist WHERE a @@ to_tsquery('footext1');

-- Verify that new inserts can be found via the index
INSERT INTO ao_with_btree SELECT generate_series(1,10);
INSERT INTO ao_with_bitmap SELECT i%3 FROM generate_series(1,10)i;
INSERT INTO ao_with_gist SELECT j.res::tsvector FROM (SELECT 'footext' || i%3 AS res FROM generate_series(1,10) i) j;
INSERT INTO aoco_with_btree SELECT generate_series(1,10);
INSERT INTO aoco_with_bitmap SELECT i%3 FROM generate_series(1,10)i;
INSERT INTO aoco_with_gist SELECT j.res::tsvector FROM (SELECT 'footext' || i%3 AS res FROM generate_series(1,10) i) j;

EXPLAIN (COSTS off) SELECT * FROM ao_with_btree WHERE a > 8;
EXPLAIN (COSTS off) SELECT * FROM ao_with_bitmap WHERE a = 1;
EXPLAIN (COSTS off) SELECT * FROM ao_with_gist WHERE a @@ to_tsquery('footext1');
EXPLAIN (COSTS off) SELECT * FROM aoco_with_btree WHERE a > 8;
EXPLAIN (COSTS off) SELECT * FROM aoco_with_bitmap WHERE a = 1;
EXPLAIN (COSTS off) SELECT * FROM aoco_with_gist WHERE a @@ to_tsquery('footext1');
SELECT * FROM ao_with_btree WHERE a > 8;
SELECT * FROM ao_with_bitmap WHERE a = 1;
SELECT * FROM ao_with_gist WHERE a @@ to_tsquery('footext1');
SELECT * FROM aoco_with_btree WHERE a > 8;
SELECT * FROM aoco_with_bitmap WHERE a = 1;
SELECT * FROM aoco_with_gist WHERE a @@ to_tsquery('footext1');

-- Verify that updates can be found via the index
UPDATE ao_with_btree SET a = 11 WHERE a = 9;
UPDATE ao_with_bitmap SET a = 4 WHERE a = 1;
UPDATE ao_with_gist SET a = 'footext5' WHERE a = 'footext1';
UPDATE aoco_with_btree SET a = 11 WHERE a = 9;
UPDATE aoco_with_bitmap SET a = 4 WHERE a = 1;
UPDATE aoco_with_gist SET a = 'footext5' WHERE a = 'footext1';

EXPLAIN (COSTS off) SELECT * FROM ao_with_btree WHERE a > 8;
EXPLAIN (COSTS off) SELECT * FROM ao_with_bitmap WHERE a = 1;
EXPLAIN (COSTS off) SELECT * FROM ao_with_gist WHERE a @@ to_tsquery('footext1');
EXPLAIN (COSTS off) SELECT * FROM aoco_with_btree WHERE a > 8;
EXPLAIN (COSTS off) SELECT * FROM aoco_with_bitmap WHERE a = 1;
EXPLAIN (COSTS off) SELECT * FROM aoco_with_gist WHERE a @@ to_tsquery('footext1');
SELECT * FROM ao_with_btree WHERE a > 8;
SELECT * FROM ao_with_bitmap WHERE a = 4;
SELECT * FROM ao_with_gist WHERE a @@ to_tsquery('footext5');
SELECT * FROM aoco_with_btree WHERE a > 8;
SELECT * FROM aoco_with_bitmap WHERE a = 4;
SELECT * FROM aoco_with_gist WHERE a @@ to_tsquery('footext5');



-- Check unused aoblkdir edge case is filtered out and not upgraded
SELECT c.relname AS relname,
CASE
	WHEN a.blkdirrelid = 0 THEN 'False'
	ELSE 'True'
END AS has_aoblkdir
FROM pg_appendonly a
JOIN pg_class c on c.oid=a.relid
WHERE c.relname='aotable_with_all_indexes_dropped';
