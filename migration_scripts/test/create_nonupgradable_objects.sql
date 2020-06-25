-- Copyright (c) 2017-2020 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

DROP TABLE IF EXISTS regular CASCADE;
CREATE TABLE regular (a int unique);

-- create partitioned table with foreign, unique and primary key constraints
DROP TABLE IF EXISTS pt_with_index CASCADE;
CREATE TABLE pt_with_index (a int references regular(a), b int, c int, d int)
    PARTITION BY RANGE(b)
        (
        PARTITION pt1 START(1),
        PARTITION pt2 START(2) END (3)
        );

CREATE INDEX ptidxc on pt_with_index(c);
CREATE INDEX ptidxb_prt_2 on pt_with_index_1_prt_pt2(b);
CREATE INDEX ptidxc_prt_2 on pt_with_index_1_prt_pt2(c);
INSERT INTO pt_with_index SELECT i, i%2+1, i, i FROM generate_series(1,10)i;

-- create tables where the index relation name is not equal primary/unique key constraint name
CREATE TYPE table_with_unique_constraint_author_key AS (dummy int);
CREATE TYPE table_with_unique_constraint_author_key1 AS (dummy int);
CREATE TABLE table_with_unique_constraint (author int, title int, CONSTRAINT table_with_unique_constraint_uniq_au_ti UNIQUE (author, title)) DISTRIBUTED BY (author);
DROP TYPE table_with_unique_constraint_author_key, table_with_unique_constraint_author_key1;
ALTER TABLE table_with_unique_constraint ADD PRIMARY KEY (author, title);
INSERT INTO table_with_unique_constraint VALUES(1, 1);
INSERT INTO table_with_unique_constraint VALUES(2, 2);

CREATE TYPE table_with_primary_constraint_pkey AS (dummy int);
CREATE TYPE table_with_primary_constraint_pkey1 AS (dummy int);
CREATE TABLE table_with_primary_constraint (author int, title int, CONSTRAINT table_with_primary_constraint_au_ti PRIMARY KEY (author, title)) DISTRIBUTED BY (author);
DROP TYPE table_with_primary_constraint_pkey, table_with_primary_constraint_pkey1;
ALTER TABLE table_with_primary_constraint ADD UNIQUE (author, title);
INSERT INTO table_with_primary_constraint VALUES(1, 1);
INSERT INTO table_with_primary_constraint VALUES(2, 2);

-- create role with gphdfs readable and writable privileges
CREATE ROLE gphdfs_user CREATEEXTTABLE(protocol='gphdfs', type='writable') CREATEEXTTABLE(protocol='gphdfs', type='readable');
