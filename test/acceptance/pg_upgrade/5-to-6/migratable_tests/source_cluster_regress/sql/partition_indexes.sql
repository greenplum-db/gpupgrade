-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

--------------------------------------------------------------------------------
-- Create and setup migratable objects
--------------------------------------------------------------------------------

-- simple partition tables with indexes
CREATE TABLE test_scores (student_id int, score int)
PARTITION BY RANGE (score)
(
    START (0) INCLUSIVE
    END (60) EXCLUSIVE,
    START (60) INCLUSIVE
    END (70) EXCLUSIVE,
    START (70) INCLUSIVE
    END (80) EXCLUSIVE,
    START (80) INCLUSIVE
    END (90) EXCLUSIVE,
    START (90) INCLUSIVE
    END (101) EXCLUSIVE
);
CREATE INDEX test_scores_idx ON test_scores(score);

INSERT INTO test_scores VALUES (1, 50);
INSERT INTO test_scores VALUES (2, 60);
INSERT INTO test_scores VALUES (3, 70);
INSERT INTO test_scores VALUES (4, 80);
INSERT INTO test_scores VALUES (5, 90);

-- create multi level partitioned table with indexes
CREATE TABLE sales (
    trans_id int,
    office_id int,
    region text
) DISTRIBUTED BY (trans_id)
PARTITION BY RANGE (office_id)
    SUBPARTITION BY LIST (region)
        SUBPARTITION TEMPLATE (
            SUBPARTITION usa VALUES ('usa'),
            SUBPARTITION asia VALUES ('asia'),
            SUBPARTITION europe VALUES ('europe'),
            DEFAULT SUBPARTITION other_regions
        )
    (
        START (1) END (4) EVERY (1),
        DEFAULT PARTITION outlying_dates
    );

CREATE INDEX sales_idx on sales(office_id);
CREATE INDEX sales_idx_bitmap on sales using bitmap(office_id);
CREATE INDEX sales_1_prt_2_idx on sales_1_prt_2(office_id, region);
CREATE INDEX sales_1_prt_3_2_prt_asia_idx on sales_1_prt_3_2_prt_asia(region);
CREATE INDEX sales_1_prt_outlying_dates_idx on sales_1_prt_outlying_dates(trans_id);
CREATE UNIQUE INDEX sales_unique_idx on sales(trans_id);

INSERT INTO sales VALUES (1, 1, 'asia');
INSERT INTO sales VALUES (1, 2, 'asia');
INSERT INTO sales VALUES (1, 3, 'asia');
INSERT INTO sales VALUES (2, 1, 'europe');
INSERT INTO sales VALUES (2, 2, 'europe');
INSERT INTO sales VALUES (2, 3, 'europe');

-- check root partition index
SELECT indrelid::regclass AS table_name,
       unnest(indkey) AS column_num
FROM pg_index pi
JOIN pg_partition pp ON pi.indrelid = pp.parrelid
JOIN pg_class pc ON pc.oid = pp.parrelid
WHERE pc.relname = 'test_scores' OR pc.relname = 'sales'
ORDER BY 1, 2;

-- check child partition indexes
SELECT indrelid::regclass AS table_name,
       unnest(indkey) AS column_num
FROM pg_index pi
JOIN pg_partition_rule pp ON pi.indrelid=pp.parchildrelid
JOIN pg_class pc ON pc.oid=pp.parchildrelid
WHERE pc.relname LIKE 'test_scores%'
    OR pc.relname LIKE 'sales%'
	AND pc.relhassubclass='f'
ORDER by 1, 2;

-- check data
SELECT * FROM test_scores ORDER BY 1, 2;
SELECT * FROM sales ORDER BY 1, 2, 3;
