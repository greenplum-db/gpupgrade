-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- Test to ensure that AOCO tables with multiple segfiles can be upgraded
-- successfully. Multiple sessions are utilized to create multiple segfiles.

--------------------------------------------------------------------------------
-- Create and setup upgradeable objects
--------------------------------------------------------------------------------

-- AOCO table with multiple segment files
CREATE TABLE aoco_multi_segment (id integer, name text) WITH (appendonly=true, orientation=column) DISTRIBUTED BY (id);

1:BEGIN;
2:BEGIN;
1:INSERT INTO aoco_multi_segment VALUES (1, 'Jane');
1:INSERT INTO aoco_multi_segment VALUES (2, 'John');
2:INSERT INTO aoco_multi_segment VALUES (3, 'Joe');
1:END;
2:END;

SELECT * FROM aoco_multi_segment ORDER BY id;
SELECT segno, column_num, physical_segno, tupcount, eof, eof_uncompressed, modcount, formatversion, state FROM gp_toolkit.__gp_aocsseg_name('aoco_multi_segment') ORDER BY 1, 2;
SELECT gp_segment_id, (t).segno, (t).column_num, (t).physical_segno, (t).tupcount, (t).eof, (t).eof_uncompressed, (t).modcount, (t).formatversion, (t).state FROM (SELECT gp_segment_id, gp_toolkit.__gp_aocsseg_name('aoco_multi_segment') AS t FROM gp_dist_random('gp_id')) AS x ORDER BY 1, 2, 3;



-- AOCO table with deleted rows that meets compaction threshold
CREATE TABLE aoco_vacuum_compact_after_upgrade (a int, b int) WITH (appendonly=true, orientation=column);

1: BEGIN;
1: INSERT INTO aoco_vacuum_compact_after_upgrade SELECT i, i FROM generate_series(1,10)i;
2: INSERT INTO aoco_vacuum_compact_after_upgrade SELECT i, i FROM generate_series(11,20)i;
1: COMMIT;
DELETE FROM aoco_vacuum_compact_after_upgrade WHERE a > 5;

SELECT * FROM aoco_vacuum_compact_after_upgrade ORDER BY a;
SELECT segno, column_num, physical_segno, tupcount, eof, eof_uncompressed, modcount, formatversion, state FROM gp_toolkit.__gp_aocsseg_name('aoco_vacuum_compact_after_upgrade') ORDER BY 1, 2;
SELECT gp_segment_id, (t).segno, (t).column_num, (t).physical_segno, (t).tupcount, (t).eof, (t).eof_uncompressed, (t).modcount, (t).formatversion, (t).state FROM (SELECT gp_segment_id, gp_toolkit.__gp_aocsseg_name('aoco_vacuum_compact_after_upgrade') AS t FROM gp_dist_random('gp_id')) AS x ORDER BY 1, 2, 3;



-- AOCO table with an AO segment in awaiting drop state
CREATE TABLE aoco_with_awaiting_drop_state_before_upgrade (a int, b int) WITH (appendonly=true, orientation=column);
INSERT INTO aoco_with_awaiting_drop_state_before_upgrade SELECT i, i FROM generate_series(1,10)i;
DELETE FROM aoco_with_awaiting_drop_state_before_upgrade;

1: BEGIN;
1: SELECT * FROM aoco_with_awaiting_drop_state_before_upgrade ORDER BY a;
2: VACUUM aoco_with_awaiting_drop_state_before_upgrade;
1: END;

SELECT * FROM aoco_with_awaiting_drop_state_before_upgrade ORDER BY a;
SELECT segno, column_num, physical_segno, tupcount, eof, eof_uncompressed, modcount, formatversion, state FROM gp_toolkit.__gp_aocsseg_name('aoco_with_awaiting_drop_state_before_upgrade') ORDER BY 1, 2;
SELECT gp_segment_id, (t).segno, (t).column_num, (t).physical_segno, (t).tupcount, (t).eof, (t).eof_uncompressed, (t).modcount, (t).formatversion, (t).state FROM (SELECT gp_segment_id, gp_toolkit.__gp_aocsseg_name('aoco_with_awaiting_drop_state_before_upgrade') AS t FROM gp_dist_random('gp_id')) AS x ORDER BY 1, 2, 3;

INSERT INTO aoco_with_awaiting_drop_state_before_upgrade SELECT i, i FROM generate_series(1,10)i;

SELECT * FROM aoco_with_awaiting_drop_state_before_upgrade ORDER BY a;
SELECT segno, column_num, physical_segno, tupcount, eof, eof_uncompressed, modcount, formatversion, state FROM gp_toolkit.__gp_aocsseg_name('aoco_with_awaiting_drop_state_before_upgrade') ORDER BY 1, 2;
SELECT gp_segment_id, (t).segno, (t).column_num, (t).physical_segno, (t).tupcount, (t).eof, (t).eof_uncompressed, (t).modcount, (t).formatversion, (t).state FROM (SELECT gp_segment_id, gp_toolkit.__gp_aocsseg_name('aoco_with_awaiting_drop_state_before_upgrade') AS t FROM gp_dist_random('gp_id')) AS x ORDER BY 1, 2, 3;



-- AOCO table with empty AO segments
CREATE TABLE aoco_with_empty_aosegs_before_upgrade (a int) WITH (appendonly=true, orientation=column);

1: BEGIN;
2: BEGIN;
1: INSERT INTO aoco_with_empty_aosegs_before_upgrade SELECT generate_series(1,10);
2: INSERT INTO aoco_with_empty_aosegs_before_upgrade SELECT generate_series(1,10);
3: INSERT INTO aoco_with_empty_aosegs_before_upgrade SELECT generate_series(11,20);
1: COMMIT;
2: COMMIT;
DELETE FROM aoco_with_empty_aosegs_before_upgrade;
VACUUM aoco_with_empty_aosegs_before_upgrade;

SELECT * FROM aoco_with_empty_aosegs_before_upgrade ORDER BY a;
SELECT segno, column_num, physical_segno, tupcount, eof, eof_uncompressed, modcount, formatversion, state FROM gp_toolkit.__gp_aocsseg_name('aoco_with_empty_aosegs_before_upgrade') ORDER BY 1, 2;
SELECT gp_segment_id, (t).segno, (t).column_num, (t).physical_segno, (t).tupcount, (t).eof, (t).eof_uncompressed, (t).modcount, (t).formatversion, (t).state FROM (SELECT gp_segment_id, gp_toolkit.__gp_aocsseg_name('aoco_with_empty_aosegs_before_upgrade') AS t FROM gp_dist_random('gp_id')) AS x ORDER BY 1, 2, 3;
