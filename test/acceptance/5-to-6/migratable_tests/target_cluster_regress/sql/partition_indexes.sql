-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

--------------------------------------------------------------------------------
-- Create and setup migratable objects
--------------------------------------------------------------------------------

-- check data
SELECT * FROM test_scores ORDER BY 1, 2;
SELECT * FROM sales ORDER BY 1, 2, 3;

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

-- insert data
INSERT INTO test_scores VALUES (6, 51);
INSERT INTO test_scores VALUES (7, 61);
INSERT INTO test_scores VALUES (8, 71);
INSERT INTO test_scores VALUES (9, 81);
INSERT INTO test_scores VALUES (10, 91);

INSERT INTO sales VALUES (3, 1, 'usa');
INSERT INTO sales VALUES (3, 2, 'usa');
INSERT INTO sales VALUES (3, 3, 'usa');
INSERT INTO sales VALUES (4, 1, 'zzz');
INSERT INTO sales VALUES (4, 2, 'zzz');
INSERT INTO sales VALUES (4, 3, 'zzz');

-- check data
SELECT * FROM test_scores ORDER BY 1, 2;
SELECT * FROM sales ORDER BY 1, 2, 3;
