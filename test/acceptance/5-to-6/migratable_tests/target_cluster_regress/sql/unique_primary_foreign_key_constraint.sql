-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

--------------------------------------------------------------------------------
-- Create and setup migratable objects
--------------------------------------------------------------------------------
SET search_path TO constraints;

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

-- insert data and exercise constraint
INSERT INTO fk_pt_with_index VALUES (3, 3, 3, 3);
INSERT INTO fk_pt_with_index VALUES (3, 3, 3, 3);

-- check data
SELECT * FROM fk_pt_with_index ORDER BY 1, 2, 3, 4;



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

-- insert data and exercise constraint
INSERT INTO table_with_unique_constraint VALUES (3, 3);
INSERT INTO table_with_unique_constraint VALUES (3, 3);
INSERT INTO table_with_unique_constraint_p VALUES (3, 3);
INSERT INTO table_with_unique_constraint_p VALUES (3, 3);

-- check data
SELECT * FROM table_with_unique_constraint ORDER BY 1, 2;
SELECT * FROM table_with_unique_constraint_p ORDER BY 1, 2;



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

-- insert data and exercise constraint
INSERT INTO table_with_primary_constraint VALUES (3, 3);
INSERT INTO table_with_primary_constraint VALUES (3, 3);
INSERT INTO table_with_primary_constraint_p VALUES (3, 3);
INSERT INTO table_with_primary_constraint_p VALUES (3, 3);

-- check data
SELECT * FROM table_with_primary_constraint ORDER BY 1, 2;
SELECT * FROM table_with_primary_constraint_p ORDER BY 1, 2;
