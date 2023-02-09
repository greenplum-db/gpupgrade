-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

--------------------------------------------------------------------------------
-- Validate gp_fastsequence was upgraded
--------------------------------------------------------------------------------

-- Verify table's gp_fastsequence value is preserved
SELECT fs.gp_segment_id, fs.objmod, fs.last_sequence
FROM pg_class c
JOIN pg_appendonly ao ON c.oid=ao.relid
JOIN gp_dist_random('gp_fastsequence') fs ON ao.segrelid=fs.objid
WHERE c.relname='aotable_fastsequence'
ORDER BY 1, 2, 3;

-- Verify table data is not corrupt using seqscan
SET enable_indexscan = false;
SET enable_bitmapscan = false;
SET enable_seqscan = true;
SELECT * FROM aotable_fastsequence ORDER BY i;

-- Verify INSERTs produce no duplicate ctids
1: BEGIN;
1: INSERT INTO aotable_fastsequence SELECT generate_series(1001, 1010);
2: INSERT INTO aotable_fastsequence SELECT generate_series(1011, 1020);
1: COMMIT;
SELECT gp_segment_id, ctid, count(ctid) FROM aotable_fastsequence GROUP BY gp_segment_id, ctid HAVING count(ctid) > 1;

-- The following gpdb commits changed aotids format which means indexes are not
-- safe to upgrade.
-- https://github.com/greenplum-db/gpdb/commit/c249ac7a36d9da3d25b6c419fbd07e2c9cfe954f
-- https://github.com/greenplum-db/gpdb/commit/fa1e76c3d72316bdcb34dd3d3b34736cd03e840f
-- Indexes are invalid and should not work after upgrade
-- This select will cause an expected FATAL that will trigger a crash recovery.
SET enable_indexscan = true;
SET enable_bitmapscan = true;
SET enable_seqscan = false;
SET gp_debug_linger = 0;
SELECT * FROM aotable_fastsequence WHERE i < 10 ORDER BY i;

-- Verify indexes are functional after REINDEX
REINDEX TABLE aotable_fastsequence;
SELECT * FROM aotable_fastsequence WHERE i < 10 ORDER BY i;



-- Verify table's gp_fastsequence value is preserved
SELECT fs.gp_segment_id, fs.objmod, fs.last_sequence
FROM pg_class c
JOIN pg_appendonly ao ON c.oid=ao.relid
JOIN gp_dist_random('gp_fastsequence') fs ON ao.segrelid=fs.objid
WHERE c.relname='aocotable_fastsequence'
ORDER BY 1, 2, 3;

-- Verify table data is not corrupt using seqscan
SET enable_indexscan = false;
SET enable_bitmapscan = false;
SET enable_seqscan = true;
SELECT * FROM aocotable_fastsequence ORDER BY i;

-- Verify INSERTs produce no duplicate ctids
-- Verify using additional sessions since sessions 1 and 2 become disconnected
-- due to the expected failed index query in session 1 causing a crash recovery.
3: BEGIN;
3: INSERT INTO aocotable_fastsequence SELECT generate_series(1001, 1010);
4: INSERT INTO aocotable_fastsequence SELECT generate_series(1011, 1020);
3: COMMIT;
SELECT gp_segment_id, ctid, count(ctid) FROM aocotable_fastsequence GROUP BY gp_segment_id, ctid HAVING count(ctid) > 1;

-- The following gpdb commits changed aotids format which means indexes are not
-- safe to upgrade.
-- https://github.com/greenplum-db/gpdb/commit/c249ac7a36d9da3d25b6c419fbd07e2c9cfe954f
-- https://github.com/greenplum-db/gpdb/commit/fa1e76c3d72316bdcb34dd3d3b34736cd03e840f
-- Indexes are invalid and should not work after upgrade
-- This select will cause an expected FATAL that will trigger a crash recovery.
SET enable_indexscan = true;
SET enable_bitmapscan = true;
SET enable_seqscan = false;
SET gp_debug_linger = 0;
SELECT * FROM aocotable_fastsequence WHERE i < 10 ORDER BY i;

-- Verify indexes are functional after REINDEX
REINDEX TABLE aocotable_fastsequence;
SELECT * FROM aocotable_fastsequence WHERE i < 10 ORDER BY i;
