-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- Test to ensure that gp_fastsequence values for ao tables get upgraded.

--------------------------------------------------------------------------------
-- Create and setup upgradeable objects
--------------------------------------------------------------------------------

-- gp_fastsequence reserves values in multiples of 100 with a minimum of 100
-- values for ctids on insert. Multiple inserts of less than 100 values may
-- result in duplicate ctids if gp_fastsequence if not preserved during
-- upgrade.
CREATE TABLE aotable_fastsequence (i int) WITH (appendonly=true);
1: BEGIN;
1: INSERT INTO aotable_fastsequence SELECT generate_series(1,10);
1: INSERT INTO aotable_fastsequence SELECT generate_series(11,20);
1: INSERT INTO aotable_fastsequence SELECT generate_series(21,30);
2: INSERT INTO aotable_fastsequence SELECT generate_series(102,121);
1: COMMIT;
CREATE INDEX aotable_fastsequence_idx ON aotable_fastsequence USING btree(i);

-- Verify table's gp_fastsequence
SELECT fs.gp_segment_id, fs.objmod, fs.last_sequence
FROM pg_class c
JOIN pg_appendonly ao ON c.oid=ao.relid
JOIN gp_dist_random('gp_fastsequence') fs ON ao.segrelid=fs.objid
WHERE c.relname='aotable_fastsequence'
ORDER BY 1, 2, 3;

CREATE TABLE aocotable_fastsequence (i int) WITH (appendonly=true, orientation=column);
1: BEGIN;
1: INSERT INTO aocotable_fastsequence SELECT generate_series(1,10);
1: INSERT INTO aocotable_fastsequence SELECT generate_series(11,20);
1: INSERT INTO aocotable_fastsequence SELECT generate_series(21,30);
2: INSERT INTO aocotable_fastsequence SELECT generate_series(102,121);
1: COMMIT;
CREATE INDEX aocotable_fastsequence_idx ON aocotable_fastsequence USING btree(i);

-- Verify table's gp_fastsequence
SELECT fs.gp_segment_id, fs.objmod, fs.last_sequence
FROM pg_class c
JOIN pg_appendonly ao ON c.oid=ao.relid
JOIN gp_dist_random('gp_fastsequence') fs ON ao.segrelid=fs.objid
WHERE c.relname='aocotable_fastsequence'
ORDER BY 1, 2, 3;
