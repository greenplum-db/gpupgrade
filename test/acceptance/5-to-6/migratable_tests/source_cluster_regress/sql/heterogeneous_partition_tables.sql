-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

--------------------------------------------------------------------------------
-- Create and setup migratable objects
--------------------------------------------------------------------------------

-- Heterogeneous partition table with dropped column, constraint, and default
-- The root and only a subset of children have the dropped column reference.
CREATE TABLE dropped_column (
    a int CONSTRAINT positive_int CHECK (b > 0),
    b int DEFAULT 1,
    c char,
    d varchar(50)
) DISTRIBUTED BY (c)
PARTITION BY RANGE (a)
(
    PARTITION part_1 START(1) END(5),
    PARTITION part_2 START(5)
);

ALTER TABLE dropped_column DROP COLUMN d;
ALTER TABLE dropped_column OWNER TO test_role1;

-- Splitting the subpartition leads to its rewrite, eliminating its dropped column
-- reference. So, after this, only part_2 and the root partition will have a
-- dropped column reference.
ALTER TABLE dropped_column SPLIT PARTITION FOR (1) AT (2)
INTO (
    PARTITION split_part_1,
    PARTITION split_part_2
);

INSERT INTO dropped_column VALUES (1, 1, 'a');
INSERT INTO dropped_column VALUES (5, 1, 'a');

-- Root partitions do not have dropped column references, but some child partitions do
CREATE TABLE child_has_dropped_column (
    a int,
    b int,
    c char,
    d varchar(50)
) PARTITION BY RANGE (a)
(
    PARTITION part_1 START(1) END(5),
    PARTITION part_2 START(5)
);

CREATE TABLE intermediate_table (
    a int,
    b int,
    c char,
    d varchar(50),
    to_drop int
);
ALTER TABLE intermediate_table DROP COLUMN to_drop;

ALTER TABLE child_has_dropped_column EXCHANGE PARTITION part_1 WITH TABLE intermediate_table;

DROP TABLE intermediate_table;

INSERT INTO child_has_dropped_column VALUES (1, 1, 'a', 'aaa');

-- heterogeneous multilevel partitioned table
CREATE TABLE heterogeneous_ml_partition_table (
    trans_id int,
    office_id int,
    region int,
    dummy int
) DISTRIBUTED BY (trans_id)
PARTITION BY RANGE (office_id)
    SUBPARTITION BY RANGE (dummy)
        SUBPARTITION TEMPLATE (
            START (1) END (16) EVERY (4),
            DEFAULT SUBPARTITION other_dummy
        )
    (
        START (1) END (4) EVERY (1),
        DEFAULT PARTITION outlying_dates
    );

ALTER TABLE heterogeneous_ml_partition_table DROP COLUMN region;
ALTER TABLE heterogeneous_ml_partition_table
ALTER PARTITION FOR (1) SPLIT PARTITION FOR (1) AT (3)
INTO (
    PARTITION p1,
    PARTITION p2
);

INSERT INTO heterogeneous_ml_partition_table VALUES (1, 1, 1);
INSERT INTO heterogeneous_ml_partition_table VALUES (2, 2, 2);

-- check data
SELECT * FROM dropped_column ORDER BY 1, 2, 3;
SELECT * FROM child_has_dropped_column ORDER BY 1, 2, 3, 4;
SELECT * FROM heterogeneous_ml_partition_table ORDER BY 1, 2, 3;

-- check owners
SELECT c.relname, pg_catalog.pg_get_userbyid(c.relowner)
FROM pg_partition_rule pr
JOIN pg_class c ON c.oid = pr.parchildrelid
WHERE c.relname LIKE 'dropped_column%'
UNION
SELECT c.relname, pg_catalog.pg_get_userbyid(c.relowner)
FROM pg_partition p
JOIN pg_class c ON c.oid = p.parrelid
WHERE c.relname LIKE 'dropped_column%'
ORDER BY 1,2;

-- check constraints
SELECT c.relname, con.conname
FROM pg_partition_rule pr
JOIN pg_class c ON c.oid = pr.parchildrelid
JOIN pg_constraint con ON con.conrelid = c.oid
WHERE c.relname LIKE 'dropped_column%'
UNION
SELECT c.relname, con.conname
FROM pg_partition p
JOIN pg_class c ON c.oid = p.parrelid
JOIN pg_constraint con ON con.conrelid = c.oid
WHERE c.relname LIKE 'dropped_column%'
ORDER BY 1,2;

-- check defaults
SELECT c.relname, att.attname, ad.adnum, ad.adsrc
FROM pg_partition_rule pr
JOIN pg_class c ON c.oid = pr.parchildrelid
JOIN pg_attrdef ad ON ad.adrelid = pr.parchildrelid
JOIN pg_attribute att ON att.attrelid = c.oid AND att.attnum = ad.adnum
UNION
SELECT c.relname, att.attname, ad.adnum, ad.adsrc
FROM pg_partition p
JOIN pg_class c ON c.oid = p.parrelid
JOIN pg_attrdef ad ON ad.adrelid = p.parrelid
JOIN pg_attribute att ON att.attrelid = c.oid AND att.attnum = ad.adnum
ORDER BY 1, 2, 3, 4;
