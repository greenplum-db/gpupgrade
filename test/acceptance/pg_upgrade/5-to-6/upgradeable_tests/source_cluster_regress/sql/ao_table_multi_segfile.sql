-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- Test to ensure that AO tables with multiple segfiles can be upgraded
-- successfully. Multiple sessions are utilized to create multiple segfiles.

--------------------------------------------------------------------------------
-- Create and setup upgradeable objects
--------------------------------------------------------------------------------

-- AO table with multiple segment files
CREATE TABLE ao_multi_segment (id integer, name text) WITH (appendonly=true) DISTRIBUTED BY (id);

1:BEGIN;
2:BEGIN;
1:INSERT INTO ao_multi_segment VALUES (1, 'Jane');
1:INSERT INTO ao_multi_segment VALUES (2, 'John');
2:INSERT INTO ao_multi_segment VALUES (3, 'Joe');
1:END;
2:END;

SELECT * FROM ao_multi_segment ORDER BY id;
SELECT * FROM gp_toolkit.__gp_aoseg_name('ao_multi_segment') ORDER BY segno;
SELECT gp_segment_id, (t).* FROM (SELECT gp_segment_id, gp_toolkit.__gp_aoseg_name('ao_multi_segment') AS t FROM gp_dist_random('gp_id')) AS x;



-- AO table with deleted rows that meets compaction threshold
CREATE TABLE ao_vacuum_compact_after_upgrade (a int, b int) WITH (appendonly=true);

1: BEGIN;
1: INSERT INTO ao_vacuum_compact_after_upgrade SELECT i, i FROM generate_series(1,10)i;
2: INSERT INTO ao_vacuum_compact_after_upgrade SELECT i, i FROM generate_series(11,20)i;
1: COMMIT;
DELETE FROM ao_vacuum_compact_after_upgrade WHERE a > 5;

SELECT * FROM ao_vacuum_compact_after_upgrade ORDER BY a;
SELECT * FROM gp_toolkit.__gp_aoseg_name('ao_vacuum_compact_after_upgrade');
SELECT gp_segment_id, (t).* FROM (SELECT gp_segment_id, gp_toolkit.__gp_aoseg_name('ao_vacuum_compact_after_upgrade') AS t FROM gp_dist_random('gp_id')) AS x;



-- AO table with an AO segment in awaiting drop state
CREATE TABLE ao_with_awaiting_drop_state_before_upgrade (a int, b int) WITH (appendonly=true);
INSERT INTO ao_with_awaiting_drop_state_before_upgrade SELECT i, i FROM generate_series(1,10)i;
DELETE FROM ao_with_awaiting_drop_state_before_upgrade;

1: BEGIN;
1: SELECT * FROM ao_with_awaiting_drop_state_before_upgrade ORDER BY a;
2: VACUUM ao_with_awaiting_drop_state_before_upgrade;
1: END;

SELECT * FROM ao_with_awaiting_drop_state_before_upgrade ORDER BY a;
SELECT * FROM gp_toolkit.__gp_aoseg_name('ao_with_awaiting_drop_state_before_upgrade');
SELECT gp_segment_id, (t).* FROM (SELECT gp_segment_id, gp_toolkit.__gp_aoseg_name('ao_with_awaiting_drop_state_before_upgrade') AS t FROM gp_dist_random('gp_id')) AS x;

INSERT INTO ao_with_awaiting_drop_state_before_upgrade SELECT i, i FROM generate_series(1,10)i;

SELECT * FROM ao_with_awaiting_drop_state_before_upgrade ORDER BY a;
SELECT * FROM gp_toolkit.__gp_aoseg_name('ao_with_awaiting_drop_state_before_upgrade');
SELECT gp_segment_id, (t).* FROM (SELECT gp_segment_id, gp_toolkit.__gp_aoseg_name('ao_with_awaiting_drop_state_before_upgrade') AS t FROM gp_dist_random('gp_id')) AS x;



-- AO table with empty AO segments
CREATE TABLE ao_with_empty_aosegs_before_upgrade (a int) WITH (appendonly=true);

1: BEGIN;
2: BEGIN;
1: INSERT INTO ao_with_empty_aosegs_before_upgrade SELECT generate_series(1,10);
2: INSERT INTO ao_with_empty_aosegs_before_upgrade SELECT generate_series(1,10);
3: INSERT INTO ao_with_empty_aosegs_before_upgrade SELECT generate_series(11,20);
1: COMMIT;
2: COMMIT;
DELETE FROM ao_with_empty_aosegs_before_upgrade;
VACUUM ao_with_empty_aosegs_before_upgrade;

SELECT * FROM ao_with_empty_aosegs_before_upgrade ORDER BY a;
SELECT * FROM gp_toolkit.__gp_aoseg_name('ao_with_empty_aosegs_before_upgrade');
SELECT gp_segment_id, (t).* FROM (SELECT gp_segment_id, gp_toolkit.__gp_aoseg_name('ao_with_empty_aosegs_before_upgrade') AS t FROM gp_dist_random('gp_id')) AS x;
