-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- Test to verify distribution keys and data distribution are preserved after
-- upgrade. Since there is a new hasing method in GPDB6 (consistent jump hash),
-- we are expecting all hash operators to be turned into legacy hash operators.

--------------------------------------------------------------------------------
-- Create and setup upgradeable objects
--------------------------------------------------------------------------------

-- single column distributed tables
CREATE TABLE single_col_dist_heap (a int2) DISTRIBUTED BY (a);
INSERT INTO single_col_dist_heap SELECT generate_series(1, 10);
CREATE TABLE single_col_dist_ao (a int4) WITH (appendonly=true) DISTRIBUTED BY (a);
INSERT INTO single_col_dist_ao SELECT generate_series(1, 10);
CREATE TABLE single_col_dist_aoco (a int8) WITH (appendonly=true, orientation=column) DISTRIBUTED BY (a);
INSERT INTO single_col_dist_aoco SELECT generate_series(1, 10);

-- check distribution policy
SELECT c.relname,
dp.attrnum AS dist_key_column,
a.attname,
op.opcname AS hashop
FROM (SELECT localoid, unnest(attrnums) AS attrnum FROM gp_distribution_policy) dp
JOIN pg_class c ON c.oid = dp.localoid
JOIN pg_attribute a ON a.attrelid = dp.localoid AND a.attnum = dp.attrnum
JOIN pg_opclass op ON op.opcintype = a.atttypid
JOIN pg_am am ON op.opcmethod = am.oid AND am.amname = 'hash'
WHERE c.relname LIKE 'single_col%'
ORDER BY 1, 2, 3;

-- check data distribution
SELECT gp_segment_id, * FROM gp_dist_random('single_col_dist_heap') order by 1, 2;
SELECT gp_segment_id, * FROM gp_dist_random('single_col_dist_ao') order by 1, 2;
SELECT gp_segment_id, * FROM gp_dist_random('single_col_dist_aoco') order by 1, 2;



-- multi column distributed tables
CREATE TABLE multi_col_dist_heap (a int2, b int2) DISTRIBUTED BY (a, b);
INSERT INTO multi_col_dist_heap SELECT a, 1 AS b FROM generate_series(1,10) a;
CREATE TABLE multi_col_dist_ao (a int4, b int4) WITH (appendonly=true) DISTRIBUTED BY (a, b);
INSERT INTO multi_col_dist_ao SELECT a, 1 AS b FROM generate_series(1,10) a;
CREATE TABLE multi_col_dist_aoco (a int8, b int8) WITH (appendonly=true, orientation=column) DISTRIBUTED BY (a, b);
INSERT INTO multi_col_dist_aoco SELECT a, 1 AS b FROM generate_series(1,10) a;

-- check distribution policy
SELECT c.relname,
dp.attrnum AS dist_key_column,
a.attname,
op.opcname AS hashop
FROM (SELECT localoid, unnest(attrnums) AS attrnum FROM gp_distribution_policy) dp
JOIN pg_class c ON c.oid = dp.localoid
JOIN pg_attribute a ON a.attrelid = dp.localoid AND a.attnum = dp.attrnum
JOIN pg_opclass op ON op.opcintype = a.atttypid
JOIN pg_am am ON op.opcmethod = am.oid AND am.amname = 'hash'
WHERE c.relname LIKE 'multi_col%'
ORDER BY 1, 2, 3;

-- check data distribution
SELECT gp_segment_id, * FROM gp_dist_random('multi_col_dist_heap') order by 1, 2, 3;
SELECT gp_segment_id, * FROM gp_dist_random('multi_col_dist_ao') order by 1, 2, 3;
SELECT gp_segment_id, * FROM gp_dist_random('multi_col_dist_aoco') order by 1, 2, 3;
