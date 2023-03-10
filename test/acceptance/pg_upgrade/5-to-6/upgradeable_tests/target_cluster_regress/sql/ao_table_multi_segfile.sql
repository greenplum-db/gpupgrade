-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

--------------------------------------------------------------------------------
-- Validate that the upgradeable objects are functional post-upgrade
--------------------------------------------------------------------------------

-- Verify AO table with multiple segment files
SELECT * FROM ao_multi_segment ORDER BY id;
SELECT * FROM gp_toolkit.__gp_aoseg('ao_multi_segment');
SELECT gp_segment_id, (t).* FROM (SELECT gp_segment_id, gp_toolkit.__gp_aoseg('ao_multi_segment') AS t FROM gp_dist_random('gp_id')) AS x;

1:BEGIN;
2:BEGIN;
1:INSERT INTO ao_multi_segment VALUES (4, 'Jude');
1:INSERT INTO ao_multi_segment VALUES (5, 'Jade');
2:INSERT INTO ao_multi_segment VALUES (6, 'Jack');
1:END;
2:END;

SELECT * FROM ao_multi_segment ORDER BY id;
SELECT * FROM gp_toolkit.__gp_aoseg('ao_multi_segment');
SELECT gp_segment_id, (t).* FROM (SELECT gp_segment_id, gp_toolkit.__gp_aoseg('ao_multi_segment') AS t FROM gp_dist_random('gp_id')) AS x;



-- Verify compaction success after upgrade
SELECT * FROM ao_vacuum_compact_after_upgrade ORDER by a;
SELECT * FROM gp_toolkit.__gp_aoseg('ao_vacuum_compact_after_upgrade');
SELECT gp_segment_id, (t).* FROM (SELECT gp_segment_id, gp_toolkit.__gp_aoseg('ao_vacuum_compact_after_upgrade') AS t FROM gp_dist_random('gp_id')) AS x;

SET gp_select_invisible = on;
SELECT * FROM ao_vacuum_compact_after_upgrade ORDER BY a;
VACUUM ao_vacuum_compact_after_upgrade;
SELECT * FROM ao_vacuum_compact_after_upgrade ORDER BY a;
SET gp_select_invisible = off;

SELECT * FROM ao_vacuum_compact_after_upgrade ORDER BY a;
SELECT * FROM gp_toolkit.__gp_aoseg('ao_vacuum_compact_after_upgrade');
SELECT gp_segment_id, (t).* FROM (SELECT gp_segment_id, gp_toolkit.__gp_aoseg('ao_vacuum_compact_after_upgrade') AS t FROM gp_dist_random('gp_id')) AS x;



-- Verify compaction success after upgrade with awaiting drop state
SELECT * FROM ao_with_awaiting_drop_state_before_upgrade ORDER BY a;
SELECT * FROM gp_toolkit.__gp_aoseg('ao_with_awaiting_drop_state_before_upgrade');
SELECT gp_segment_id, (t).* FROM (SELECT gp_segment_id, gp_toolkit.__gp_aoseg('ao_with_awaiting_drop_state_before_upgrade') AS t FROM gp_dist_random('gp_id')) AS x;

VACUUM ao_with_awaiting_drop_state_before_upgrade;

SELECT * FROM gp_toolkit.__gp_aoseg('ao_with_awaiting_drop_state_before_upgrade');
SELECT gp_segment_id, (t).* FROM (SELECT gp_segment_id, gp_toolkit.__gp_aoseg('ao_with_awaiting_drop_state_before_upgrade') AS t FROM gp_dist_random('gp_id')) AS x;

1: BEGIN;
2: BEGIN;
1: INSERT INTO ao_with_awaiting_drop_state_before_upgrade VALUES (88, 88);
2: INSERT INTO ao_with_awaiting_drop_state_before_upgrade VALUES (88, 88);
3: INSERT INTO ao_with_awaiting_drop_state_before_upgrade VALUES (88, 88);
1: COMMIT;
2: COMMIT;

SELECT * FROM ao_with_awaiting_drop_state_before_upgrade ORDER BY a;
SELECT * FROM gp_toolkit.__gp_aoseg('ao_with_awaiting_drop_state_before_upgrade');
SELECT gp_segment_id, (t).* FROM (SELECT gp_segment_id, gp_toolkit.__gp_aoseg('ao_with_awaiting_drop_state_before_upgrade') AS t FROM gp_dist_random('gp_id')) AS x;



-- Verify empty AO segments are still there and can be inserted into
SELECT * FROM gp_toolkit.__gp_aoseg('ao_with_empty_aosegs_before_upgrade');
SELECT gp_segment_id, (t).* FROM (SELECT gp_segment_id, gp_toolkit.__gp_aoseg('ao_with_empty_aosegs_before_upgrade') AS t FROM gp_dist_random('gp_id')) AS x;
SELECT * FROM ao_with_empty_aosegs_before_upgrade;

INSERT INTO ao_with_empty_aosegs_before_upgrade SELECT generate_series(1,10);

SELECT * FROM ao_with_empty_aosegs_before_upgrade ORDER BY a;
SELECT * FROM gp_toolkit.__gp_aoseg('ao_with_empty_aosegs_before_upgrade');
SELECT gp_segment_id, (t).* FROM (SELECT gp_segment_id, gp_toolkit.__gp_aoseg('ao_with_empty_aosegs_before_upgrade') AS t FROM gp_dist_random('gp_id')) AS x;
