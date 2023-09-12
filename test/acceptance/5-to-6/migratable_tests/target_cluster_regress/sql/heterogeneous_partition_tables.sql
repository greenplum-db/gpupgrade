-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

--------------------------------------------------------------------------------
-- Create and setup migratable objects
--------------------------------------------------------------------------------

-- start_ignore
-- check data
SELECT * FROM dropped_column ORDER BY 1, 2, 3;
SELECT * FROM child_has_dropped_column ORDER BY 1, 2, 3, 4;
SELECT * FROM heterogeneous_ml_partition_table ORDER BY 1, 2, 3;
-- end_ignore

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

-- insert data and exercise constraint
INSERT INTO dropped_column VALUES (2, 2, 'b');
INSERT INTO dropped_column VALUES (3, 2, 'b');
-- insert should fail due to constraint
INSERT INTO dropped_column VALUES (4, -1, 'b');

INSERT INTO child_has_dropped_column VALUES (2, 2, 'b', 'bbb');

INSERT INTO heterogeneous_ml_partition_table VALUES (3, 3, 3);

-- start_ignore
-- check data
SELECT * FROM dropped_column ORDER BY 1, 2, 3;
SELECT * FROM child_has_dropped_column ORDER BY 1, 2, 3, 4;
SELECT * FROM heterogeneous_ml_partition_table ORDER BY 1, 2, 3;
-- end_ignore
