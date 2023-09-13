-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

--------------------------------------------------------------------------------
-- Create and setup migratable objects
--------------------------------------------------------------------------------
CREATE SCHEMA constraints;
SET search_path TO constraints;

-- foreign key constraints
CREATE TABLE fk_base_table (a int unique);
CREATE TABLE fk_pt_with_index (
    a int REFERENCES fk_base_table(a),
    b int,
    c int,
    d int
) PARTITION BY RANGE(b)
(
    PARTITION pt1 START(1),
    PARTITION pt2 START(2) END(3),
    PARTITION pt3 START(3) END(4)
);

CREATE INDEX fk_pt_idx_c on fk_pt_with_index(c);
CREATE INDEX fk_pt_idx_c_bitmap on fk_pt_with_index using bitmap(c);

CREATE INDEX fk_pt_idx_b_prt_2 on fk_pt_with_index_1_prt_pt2(b);
CREATE INDEX fk_pt_idx_b_prt_2_bitmap on fk_pt_with_index_1_prt_pt2 using bitmap(b);

CREATE INDEX fk_pt_idx_c_prt_2 on fk_pt_with_index_1_prt_pt2(c);
CREATE INDEX fk_pt_idx_c_prt_2_bitmap on fk_pt_with_index_1_prt_pt2 using bitmap(c);

INSERT INTO fk_pt_with_index VALUES (1, 1, 1, 1);
INSERT INTO fk_pt_with_index VALUES (2, 2, 2, 2);

-- check foreign key constraints
WITH Partitions AS (
    SELECT DISTINCT
        p.parrelid AS oid,
        n.nspname,
        c.relname
    FROM
        pg_catalog.pg_partition p
    JOIN
        pg_catalog.pg_class c ON p.parrelid = c.oid
    JOIN
        pg_catalog.pg_namespace n ON n.oid = c.relnamespace
)
SELECT nspname, relname, conname
FROM pg_constraint cc
JOIN Partitions sub ON sub.oid = cc.conrelid
WHERE cc.contype = 'f';

-- check indexes
SELECT c.relname AS index_name
FROM pg_index i
JOIN pg_class c ON i.indexrelid = c.oid
JOIN pg_class t ON i.indrelid = t.oid
AND t.relname LIKE 'fk_pt_%';

-- check data
SELECT * FROM fk_pt_with_index ORDER BY 1, 2, 3, 4;



-- unique constraints
-- create tables where the index relation name is not equal primary/unique key
-- constraint name. we create a TYPE with the default name of the constraint
-- that would have been created to force skipping the default name
CREATE TYPE table_with_unique_constraint_author_key AS (dummy int);
CREATE TYPE table_with_unique_constraint_author_key1 AS (dummy int);
CREATE TABLE table_with_unique_constraint (
    author int,
    title int,
    CONSTRAINT table_with_unique_constraint_uniq_au_ti UNIQUE (author, title)
) DISTRIBUTED BY (author);

ALTER TABLE table_with_unique_constraint ADD PRIMARY KEY (author, title);
INSERT INTO table_with_unique_constraint VALUES (1, 1);
INSERT INTO table_with_unique_constraint VALUES (2, 2);

-- create partitioned tables where the index relation name is not equal
-- primary/unique key constraint name for the root 
-- Note that the naming of the constraint is key, not the type of constraint.
-- If the constraint is named, every partition will have the same named
-- constraint and they all can be dropped with the same command. If the
-- constraint is not named, greenplum generates a unique name for each
-- partition as well as the coordinator table. We can only drop the coordinator
-- tables constraint and the partition constraints remain in effect
CREATE TYPE unique_constraint_p_author_key AS (dummy int);
CREATE TYPE unique_constraint_p_author_key1 AS (dummy int);
CREATE TABLE table_with_unique_constraint_p (
    author int,
    title int,
    CONSTRAINT unique_constraint_p_uniq_au_ti UNIQUE (author, title)
) PARTITION BY RANGE(title) (START(1) END(4) EVERY(1));

ALTER TABLE table_with_unique_constraint_p ADD PRIMARY KEY (author, title);
INSERT INTO table_with_unique_constraint_p VALUES (1, 1);
INSERT INTO table_with_unique_constraint_p VALUES (2, 2);

-- check unique constraints
WITH non_child_partitions AS (
    SELECT oid, *
    FROM pg_class
    WHERE oid NOT IN (
        SELECT DISTINCT parchildrelid
        FROM pg_partition_rule
    )
)
SELECT n.nspname, cc.relname, conname
FROM pg_constraint con
JOIN pg_depend dep
    ON (refclassid, classid, objsubid) = ('pg_constraint'::regclass, 'pg_class'::regclass, 0)
    AND refobjid = con.oid
    AND deptype = 'i'
    AND contype IN ('u', 'p', 'x') -- 'x' is an option for GPDB6, not GPDB5
JOIN non_child_partitions c ON objid = c.oid
    AND relkind = 'i'
JOIN non_child_partitions cc ON cc.oid = con.conrelid
JOIN pg_namespace n ON (n.oid = cc.relnamespace)
WHERE cc.relname LIKE 'table_with_unique_constraint%'
ORDER BY 1, 2, 3;

-- check data
SELECT * FROM table_with_unique_constraint ORDER BY 1, 2;
SELECT * FROM table_with_unique_constraint_p ORDER BY 1, 2;



-- primary constraints
-- Create type and table with primary constraint
CREATE TYPE table_with_primary_constraint_pkey AS (dummy int);
CREATE TYPE table_with_primary_constraint_pkey1 AS (dummy int);
CREATE TABLE table_with_primary_constraint (
    author int,
    title int,
    CONSTRAINT table_with_primary_constraint_au_ti PRIMARY KEY (author, title)
) DISTRIBUTED BY (author);

ALTER TABLE table_with_primary_constraint ADD UNIQUE (author, title);
INSERT INTO table_with_primary_constraint VALUES (1, 1);
INSERT INTO table_with_primary_constraint VALUES (2, 2);

-- Create type and table with primary constraint
CREATE TYPE primary_constraint_p_pkey AS (dummy int);
CREATE TYPE primary_constraint_p_pkey1 AS (dummy int);
CREATE TABLE table_with_primary_constraint_p (
    author int,
    title int,
    CONSTRAINT primary_constraint_p_au_ti PRIMARY KEY (author, title)
) PARTITION BY RANGE(title) (START(1) END(4) EVERY(1));

ALTER TABLE table_with_primary_constraint_p ADD UNIQUE (author, title);
INSERT INTO table_with_primary_constraint_p VALUES (1, 1);
INSERT INTO table_with_primary_constraint_p VALUES (2, 2);

-- check primary unique constraints
WITH non_child_partitions AS (
    SELECT oid, *
    FROM pg_class
    WHERE oid NOT IN (
        SELECT DISTINCT parchildrelid
        FROM pg_partition_rule
    )
)
SELECT n.nspname, cc.relname, conname
FROM pg_constraint con
JOIN pg_depend dep
    ON (refclassid, classid, objsubid) = ('pg_constraint'::regclass, 'pg_class'::regclass, 0)
    AND refobjid = con.oid
    AND deptype = 'i'
    AND contype IN ('u', 'p', 'x') -- 'x' is an option for GPDB6, not GPDB5
JOIN non_child_partitions c ON objid = c.oid
    AND relkind = 'i'
JOIN non_child_partitions cc ON cc.oid = con.conrelid
JOIN pg_namespace n ON (n.oid = cc.relnamespace)
WHERE cc.relname LIKE 'table_with_primary_constraint%'
ORDER BY 1, 2, 3;

-- check data
SELECT * FROM table_with_primary_constraint ORDER BY 1, 2;
SELECT * FROM table_with_primary_constraint_p ORDER BY 1, 2;
