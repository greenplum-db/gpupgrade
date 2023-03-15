-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- Test to ensure that permutations of AO/CO tables with all
-- documented index types are upgradeable.

--------------------------------------------------------------------------------
-- Create and setup upgradeable objects
--------------------------------------------------------------------------------

CREATE TABLE ao_with_btree(a int) WITH (appendonly=true);
CREATE TABLE ao_with_bitmap(a int) WITH (appendonly=true);
CREATE TABLE ao_with_gist(a tsvector) WITH (appendonly=true);
CREATE TABLE aoco_with_btree(a int) WITH (appendonly=true, orientation=column);
CREATE TABLE aoco_with_bitmap(a int) WITH (appendonly=true, orientation=column);
CREATE TABLE aoco_with_gist(a tsvector) WITH (appendonly=true, orientation=column);

INSERT INTO ao_with_btree SELECT generate_series(1,10);
INSERT INTO ao_with_bitmap SELECT i%5 FROM generate_series(1,20)i;
INSERT INTO ao_with_gist SELECT j.res::tsvector FROM (SELECT 'footext' || i%3 AS res FROM generate_series(1,10) i) j;
INSERT INTO aoco_with_btree SELECT generate_series(1,10);
INSERT INTO aoco_with_bitmap SELECT i%5 FROM generate_series(1,20)i;
INSERT INTO aoco_with_gist SELECT j.res::tsvector FROM (SELECT 'footext' || i%3 AS res FROM generate_series(1,10) i) j;

CREATE INDEX ao_with_btree_idx ON ao_with_btree USING btree(a);
CREATE INDEX ao_with_bitmap_idx ON ao_with_bitmap USING bitmap(a);
CREATE INDEX ao_with_gist_idx ON ao_with_gist USING gist(a);
CREATE INDEX aoco_with_btree_idx ON aoco_with_btree USING btree(a);
CREATE INDEX aoco_with_bitmap_idx ON aoco_with_bitmap USING bitmap(a);
CREATE INDEX aoco_with_gist_idx ON aoco_with_gist USING gist(a);

-- Show what the indexes are before upgrade
SELECT c.relname, a.amname FROM pg_class c JOIN pg_am a ON c.relam = a.oid WHERE relname SIMILAR TO '(ao|aoco)_with_(btree|bitmap|gist)_idx';

-- Show that the indexes are usable before upgrade
SET enable_seqscan = off;
SELECT * FROM ao_with_btree WHERE a > 8;
SELECT * FROM ao_with_bitmap WHERE a = 1;
SELECT * FROM ao_with_gist WHERE a @@ to_tsquery('footext1');
SELECT * FROM aoco_with_btree WHERE a > 8;
SELECT * FROM aoco_with_bitmap WHERE a = 1;
SELECT * FROM aoco_with_gist WHERE a @@ to_tsquery('footext1');



-- When the last index on an AO table is deleted, the table's corresponding
-- pg_aoblkdir is not deleted. The aoblkdir is first created when an index is
-- created for the AO table. If an index is created for an AO table then
-- deleted, an aodblkdir relation is left on the source cluster. During
-- pg_upgrade, pg_dump will not dump an index which means the aoblkdir does not
-- get created on the target cluster. If this is not accounted for, pg_upgrade
-- will error out due to relation mismatch. To resolve this edge case,
-- pg_upgrade filters out aoblkdirs for AO tables with no indexes.

-- Setup AO table with unused aoblkdir
CREATE TABLE aotable_with_all_indexes_dropped(i int) WITH (appendonly=true);
CREATE INDEX idx on aotable_with_all_indexes_dropped(i);
DROP INDEX idx;

-- Check unused aoblkdir exists
SELECT c.relname AS relname,
CASE
	WHEN a.blkdirrelid = 0 THEN 'False'
	ELSE 'True'
END AS has_aoblkdir
FROM pg_appendonly a
JOIN pg_class c on c.oid=a.relid
WHERE c.relname='aotable_with_all_indexes_dropped';
