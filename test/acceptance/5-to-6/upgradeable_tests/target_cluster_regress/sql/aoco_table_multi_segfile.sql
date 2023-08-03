-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

--------------------------------------------------------------------------------
-- Validate that the upgradeable objects are functional post-upgrade
--------------------------------------------------------------------------------

-- Verify AOCO table with multiple segment files
SELECT * FROM aoco_multi_segment ORDER BY id;
SELECT segno, column_num, physical_segno, tupcount, eof, eof_uncompressed, modcount, formatversion, state FROM gp_toolkit.__gp_aocsseg('aoco_multi_segment') ORDER BY 1, 2;
SELECT gp_segment_id, (t).segno, (t).column_num, (t).physical_segno, (t).tupcount, (t).eof, (t).eof_uncompressed, (t).modcount, (t).formatversion, (t).state FROM (SELECT gp_segment_id, gp_toolkit.__gp_aocsseg('aoco_multi_segment') AS t FROM gp_dist_random('gp_id')) AS x ORDER BY 1, 2, 3;

1:BEGIN;
2:BEGIN;
1:INSERT INTO aoco_multi_segment VALUES (4, 'Jude');
1:INSERT INTO aoco_multi_segment VALUES (5, 'Jade');
2:INSERT INTO aoco_multi_segment VALUES (6, 'Jack');
1:END;
2:END;

SELECT * FROM aoco_multi_segment ORDER BY id;
SELECT segno, column_num, physical_segno, tupcount, eof, eof_uncompressed, modcount, formatversion, state FROM gp_toolkit.__gp_aocsseg('aoco_multi_segment') ORDER BY 1, 2;
SELECT gp_segment_id, (t).segno, (t).column_num, (t).physical_segno, (t).tupcount, (t).eof, (t).eof_uncompressed, (t).modcount, (t).formatversion, (t).state FROM (SELECT gp_segment_id, gp_toolkit.__gp_aocsseg('aoco_multi_segment') AS t FROM gp_dist_random('gp_id')) AS x ORDER BY 1, 2, 3;



-- Verify compaction success after upgrade
SELECT * FROM aoco_vacuum_compact_after_upgrade ORDER BY a;
SELECT segno, column_num, physical_segno, tupcount, eof, eof_uncompressed, modcount, formatversion, state FROM gp_toolkit.__gp_aocsseg('aoco_vacuum_compact_after_upgrade') ORDER BY 1, 2;
SELECT gp_segment_id, (t).segno, (t).column_num, (t).physical_segno, (t).tupcount, (t).eof, (t).eof_uncompressed, (t).modcount, (t).formatversion, (t).state FROM (SELECT gp_segment_id, gp_toolkit.__gp_aocsseg('aoco_vacuum_compact_after_upgrade') AS t FROM gp_dist_random('gp_id')) AS x ORDER BY 1, 2, 3;

SET gp_select_invisible = on;
SELECT * FROM aoco_vacuum_compact_after_upgrade ORDER BY a;
VACUUM aoco_vacuum_compact_after_upgrade;
SELECT * FROM aoco_vacuum_compact_after_upgrade ORDER BY a;
SET gp_select_invisible = off;

SELECT * FROM aoco_vacuum_compact_after_upgrade ORDER BY a;
SELECT segno, column_num, physical_segno, tupcount, eof, eof_uncompressed, modcount, formatversion, state FROM gp_toolkit.__gp_aocsseg('aoco_vacuum_compact_after_upgrade') ORDER BY 1, 2;
SELECT gp_segment_id, (t).segno, (t).column_num, (t).physical_segno, (t).tupcount, (t).eof, (t).eof_uncompressed, (t).modcount, (t).formatversion, (t).state FROM (SELECT gp_segment_id, gp_toolkit.__gp_aocsseg('aoco_vacuum_compact_after_upgrade') AS t FROM gp_dist_random('gp_id')) AS x ORDER BY 1, 2, 3;



-- Verify compaction success after upgrade with awaiting drop state
SELECT * FROM aoco_with_awaiting_drop_state_before_upgrade ORDER BY a;
SELECT segno, column_num, physical_segno, tupcount, eof, eof_uncompressed, modcount, formatversion, state FROM gp_toolkit.__gp_aocsseg('aoco_with_awaiting_drop_state_before_upgrade') ORDER BY 1, 2;
SELECT gp_segment_id, (t).segno, (t).column_num, (t).physical_segno, (t).tupcount, (t).eof, (t).eof_uncompressed, (t).modcount, (t).formatversion, (t).state FROM (SELECT gp_segment_id, gp_toolkit.__gp_aocsseg('aoco_with_awaiting_drop_state_before_upgrade') AS t FROM gp_dist_random('gp_id')) AS x ORDER BY 1, 2, 3;

VACUUM aoco_with_awaiting_drop_state_before_upgrade;

SELECT segno, column_num, physical_segno, tupcount, eof, eof_uncompressed, modcount, formatversion, state FROM gp_toolkit.__gp_aocsseg('aoco_with_awaiting_drop_state_before_upgrade') ORDER BY 1, 2;
SELECT gp_segment_id, (t).segno, (t).column_num, (t).physical_segno, (t).tupcount, (t).eof, (t).eof_uncompressed, (t).modcount, (t).formatversion, (t).state FROM (SELECT gp_segment_id, gp_toolkit.__gp_aocsseg('aoco_with_awaiting_drop_state_before_upgrade') AS t FROM gp_dist_random('gp_id')) AS x ORDER BY 1, 2, 3;

1: BEGIN;
2: BEGIN;
1: INSERT INTO aoco_with_awaiting_drop_state_before_upgrade VALUES (88, 88);
2: INSERT INTO aoco_with_awaiting_drop_state_before_upgrade VALUES (88, 88);
3: INSERT INTO aoco_with_awaiting_drop_state_before_upgrade VALUES (88, 88);
1: COMMIT;
2: COMMIT;

SELECT * FROM aoco_with_awaiting_drop_state_before_upgrade ORDER BY a;
SELECT segno, column_num, physical_segno, tupcount, eof, eof_uncompressed, modcount, formatversion, state FROM gp_toolkit.__gp_aocsseg('aoco_with_awaiting_drop_state_before_upgrade') ORDER BY 1, 2;
SELECT gp_segment_id, (t).segno, (t).column_num, (t).physical_segno, (t).tupcount, (t).eof, (t).eof_uncompressed, (t).modcount, (t).formatversion, (t).state FROM (SELECT gp_segment_id, gp_toolkit.__gp_aocsseg('aoco_with_awaiting_drop_state_before_upgrade') AS t FROM gp_dist_random('gp_id')) AS x ORDER BY 1, 2, 3;



-- Verify empty AO segments are still be there and can be inserted into
SELECT * FROM aoco_with_empty_aosegs_before_upgrade ORDER BY a;
SELECT segno, column_num, physical_segno, tupcount, eof, eof_uncompressed, modcount, formatversion, state FROM gp_toolkit.__gp_aocsseg('aoco_with_empty_aosegs_before_upgrade') ORDER BY 1, 2;
SELECT gp_segment_id, (t).segno, (t).column_num, (t).physical_segno, (t).tupcount, (t).eof, (t).eof_uncompressed, (t).modcount, (t).formatversion, (t).state FROM (SELECT gp_segment_id, gp_toolkit.__gp_aocsseg('aoco_with_empty_aosegs_before_upgrade') AS t FROM gp_dist_random('gp_id')) AS x ORDER BY 1, 2, 3;

INSERT INTO aoco_with_empty_aosegs_before_upgrade SELECT generate_series(1,10);

SELECT * FROM aoco_with_empty_aosegs_before_upgrade ORDER BY a;
SELECT segno, column_num, physical_segno, tupcount, eof, eof_uncompressed, modcount, formatversion, state FROM gp_toolkit.__gp_aocsseg('aoco_with_empty_aosegs_before_upgrade') ORDER BY 1, 2;
SELECT gp_segment_id, (t).segno, (t).column_num, (t).physical_segno, (t).tupcount, (t).eof, (t).eof_uncompressed, (t).modcount, (t).formatversion, (t).state FROM (SELECT gp_segment_id, gp_toolkit.__gp_aocsseg('aoco_with_empty_aosegs_before_upgrade') AS t FROM gp_dist_random('gp_id')) AS x ORDER BY 1, 2, 3;
