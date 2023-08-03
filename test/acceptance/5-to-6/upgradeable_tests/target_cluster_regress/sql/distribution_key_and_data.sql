-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

--------------------------------------------------------------------------------
-- Validate that the upgradeable objects are functional post-upgrade
--------------------------------------------------------------------------------

-- check data distribution
SELECT gp_segment_id, * FROM gp_dist_random('single_col_dist_heap') ORDER BY 1, 2;
SELECT gp_segment_id, * FROM gp_dist_random('single_col_dist_ao') ORDER BY 1, 2;
SELECT gp_segment_id, * FROM gp_dist_random('single_col_dist_aoco') ORDER BY 1, 2;

-- check distribution policy
SELECT c.relname, dp.distkey, opc.opcname
FROM gp_distribution_policy dp
JOIN pg_class c ON dp.localoid = c.oid
JOIN pg_opclass opc ON dp.distclass[0] = opc.oid
WHERE c.relname LIKE 'single_col%'
ORDER BY c.relname;

-- insert same rows int tables
INSERT INTO single_col_dist_heap SELECT generate_series(1, 10);
INSERT INTO single_col_dist_ao SELECT generate_series(1, 10);
INSERT INTO single_col_dist_aoco SELECT generate_series(1, 10);

-- check data was placed into expected segments
SELECT count(*) from single_col_dist_heap;
SELECT count(*) from single_col_dist_ao;
SELECT count(*) from single_col_dist_aoco;
SELECT gp_segment_id, * FROM gp_dist_random('single_col_dist_heap') GROUP BY 1, 2 ORDER BY 1, 2;
SELECT gp_segment_id, * FROM gp_dist_random('single_col_dist_ao') GROUP BY 1, 2 ORDER BY 1, 2;
SELECT gp_segment_id, * FROM gp_dist_random('single_col_dist_aoco') GROUP BY 1, 2 ORDER BY 1, 2;

-- reorganize with the same distribution columns
ALTER TABLE single_col_dist_heap SET WITH (reorganize=true) DISTRIBUTED BY (a);
ALTER TABLE single_col_dist_ao SET WITH (reorganize=true) DISTRIBUTED BY (a);
ALTER TABLE single_col_dist_aoco SET WITH (reorganize=true) DISTRIBUTED BY (a);

-- check data distribution
SELECT gp_segment_id, * FROM gp_dist_random('single_col_dist_heap') GROUP BY 1, 2 ORDER BY 1, 2;
SELECT gp_segment_id, * FROM gp_dist_random('single_col_dist_ao') GROUP BY 1, 2 ORDER BY 1, 2;
SELECT gp_segment_id, * FROM gp_dist_random('single_col_dist_aoco') GROUP BY 1, 2 ORDER BY 1, 2;



-- check data distribution
SELECT gp_segment_id, * FROM gp_dist_random('multi_col_dist_heap') ORDER BY 1, 2, 3;
SELECT gp_segment_id, * FROM gp_dist_random('multi_col_dist_ao') ORDER BY 1, 2, 3;
SELECT gp_segment_id, * FROM gp_dist_random('multi_col_dist_aoco') ORDER BY 1, 2, 3;

-- check distribution policy
SELECT c.relname, dp.distkey, opc0.opcname, opc1.opcname
FROM gp_distribution_policy dp
JOIN pg_class c ON dp.localoid = c.oid
JOIN pg_opclass opc0 ON dp.distclass[0] = opc0.oid
JOIN pg_opclass opc1 ON dp.distclass[1] = opc1.oid
WHERE c.relname LIKE 'multi_col%'
ORDER BY c.relname;

-- insert same rows int tables
INSERT INTO multi_col_dist_heap SELECT a, 1 AS b FROM generate_series(1,10) a;
INSERT INTO multi_col_dist_ao SELECT a, 1 AS b FROM generate_series(1,10) a;
INSERT INTO multi_col_dist_aoco SELECT a, 1 AS b FROM generate_series(1,10) a;

-- check data was placed into expected segments
SELECT count(*) from multi_col_dist_heap;
SELECT count(*) from multi_col_dist_ao;
SELECT count(*) from multi_col_dist_aoco;
SELECT gp_segment_id, * FROM gp_dist_random('multi_col_dist_heap') GROUP BY 1, 2, 3 ORDER BY 1, 2, 3;
SELECT gp_segment_id, * FROM gp_dist_random('multi_col_dist_ao') GROUP BY 1, 2, 3 ORDER BY 1, 2, 3;
SELECT gp_segment_id, * FROM gp_dist_random('multi_col_dist_aoco') GROUP BY 1, 2, 3 ORDER BY 1, 2, 3;

-- reorganize with the same distribution columns
ALTER TABLE multi_col_dist_heap SET WITH (reorganize=true) DISTRIBUTED BY (a, b);
ALTER TABLE multi_col_dist_ao SET WITH (reorganize=true) DISTRIBUTED BY (a, b);
ALTER TABLE multi_col_dist_aoco SET WITH (reorganize=true) DISTRIBUTED BY (a, b);

-- check data distribution
SELECT gp_segment_id, * FROM gp_dist_random('multi_col_dist_heap') GROUP BY 1, 2, 3 ORDER BY 1, 2, 3;
SELECT gp_segment_id, * FROM gp_dist_random('multi_col_dist_ao') GROUP BY 1, 2, 3 ORDER BY 1, 2, 3;
SELECT gp_segment_id, * FROM gp_dist_random('multi_col_dist_aoco') GROUP BY 1, 2, 3 ORDER BY 1, 2, 3;
