-- Copyright (c) 2017-2021 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- Ensure data migration scripts fully qualify objects by creating the
-- non-upgradable objects in a custom schema.
DROP SCHEMA IF EXISTS testschema CASCADE;
CREATE SCHEMA testschema;
SET search_path to testschema;

DROP TABLE IF EXISTS regular CASCADE;
CREATE TABLE regular (a int unique);

-- create partitioned table with foreign key constraints
DROP TABLE IF EXISTS pt_with_index CASCADE;
CREATE TABLE pt_with_index (a int references regular(a), b int, c int, d int)
    PARTITION BY RANGE(b)
        (
        PARTITION pt1 START(1),
        PARTITION pt2 START(2) END (3),
        PARTITION pt3 START(3) END (4)
        );

CREATE INDEX ptidxc on pt_with_index(c);
CREATE INDEX ptidxc_bitmap on pt_with_index using bitmap(c);

CREATE INDEX ptidxb_prt_2 on pt_with_index_1_prt_pt2(b);
CREATE INDEX ptidxb_prt_2_bitmap on pt_with_index_1_prt_pt2 using bitmap(b);

CREATE INDEX ptidxc_prt_2 on pt_with_index_1_prt_pt2(c);
CREATE INDEX ptidxc_prt_2_bitmap on pt_with_index_1_prt_pt2 using bitmap(c);
INSERT INTO pt_with_index SELECT i, i%2+1, i, i FROM generate_series(1,10)i;

-- create multi level partitioned table with indexes
DROP TABLE IF EXISTS sales;
CREATE TABLE sales (trans_id int, office_id int, region text)
    DISTRIBUTED BY (trans_id)
    PARTITION BY RANGE (office_id)
        SUBPARTITION BY LIST (region)
            SUBPARTITION TEMPLATE
            ( SUBPARTITION usa VALUES ('usa'),
            SUBPARTITION asia VALUES ('asia'),
            SUBPARTITION europe VALUES ('europe'),
            DEFAULT SUBPARTITION other_regions)
        (START (1) END (4) EVERY (1),
        DEFAULT PARTITION outlying_dates );

CREATE INDEX sales_idx on sales(office_id);
CREATE INDEX sales_idx_bitmap on sales using bitmap(office_id);
CREATE INDEX sales_1_prt_2_idx on sales_1_prt_2(office_id, region);
CREATE INDEX sales_1_prt_3_2_prt_asia_idx on sales_1_prt_3_2_prt_asia(region);
CREATE INDEX sales_1_prt_outlying_dates_idx on sales_1_prt_outlying_dates(trans_id);
CREATE UNIQUE INDEX sales_unique_idx on sales(office_id);

-- create tables where the index relation name is not equal primary/unique key constraint name.
-- we create a TYPE with the default name of the constraint that would have been created to force
-- skipping the default name
DROP TABLE IF EXISTS table_with_unique_constraint;
CREATE TYPE table_with_unique_constraint_author_key AS (dummy int);
CREATE TYPE table_with_unique_constraint_author_key1 AS (dummy int);
CREATE TABLE table_with_unique_constraint (author int, title int, CONSTRAINT table_with_unique_constraint_uniq_au_ti UNIQUE (author, title)) DISTRIBUTED BY (author);
DROP TYPE table_with_unique_constraint_author_key, table_with_unique_constraint_author_key1;
ALTER TABLE table_with_unique_constraint ADD PRIMARY KEY (author, title);
INSERT INTO table_with_unique_constraint VALUES(1, 1);
INSERT INTO table_with_unique_constraint VALUES(2, 2);

DROP TABLE IF EXISTS table_with_primary_constraint;
CREATE TYPE table_with_primary_constraint_pkey AS (dummy int);
CREATE TYPE table_with_primary_constraint_pkey1 AS (dummy int);
CREATE TABLE table_with_primary_constraint (author int, title int, CONSTRAINT table_with_primary_constraint_au_ti PRIMARY KEY (author, title)) DISTRIBUTED BY (author);
DROP TYPE table_with_primary_constraint_pkey, table_with_primary_constraint_pkey1;
ALTER TABLE table_with_primary_constraint ADD UNIQUE (author, title);
INSERT INTO table_with_primary_constraint VALUES(1, 1);
INSERT INTO table_with_primary_constraint VALUES(2, 2);

-- create role with gphdfs readable and writable privileges
CREATE ROLE gphdfs_user CREATEEXTTABLE(protocol='gphdfs', type='writable') CREATEEXTTABLE(protocol='gphdfs', type='readable');

-- create partitioned tables where the index relation name is not equal primary/unique key constraint name for the root
DROP TABLE IF EXISTS table_with_unique_constraint_p;
CREATE TYPE table_with_unique_constraint_p_author_key AS (dummy int);
CREATE TYPE table_with_unique_constraint_p_author_key1 AS (dummy int);
CREATE TABLE table_with_unique_constraint_p (author int, title int, CONSTRAINT table_with_unique_constraint_p_uniq_au_ti UNIQUE (author, title)) PARTITION BY RANGE(title) (START(1) END(4) EVERY(1));
DROP TYPE table_with_unique_constraint_p_author_key, table_with_unique_constraint_p_author_key1;
ALTER TABLE table_with_unique_constraint_p ADD PRIMARY KEY (author, title);
INSERT INTO table_with_unique_constraint_p VALUES(1, 1);
INSERT INTO table_with_unique_constraint_p VALUES(2, 2);

DROP TABLE IF EXISTS table_with_primary_constraint_p;
CREATE TYPE table_with_primary_constraint_p_pkey AS (dummy int);
CREATE TYPE table_with_primary_constraint_p_pkey1 AS (dummy int);
CREATE TABLE table_with_primary_constraint_p (author int, title int, CONSTRAINT table_with_primary_constraint_p_au_ti PRIMARY KEY (author, title)) PARTITION BY RANGE(title) (START(1) END(4) EVERY(1));
DROP TYPE table_with_primary_constraint_p_pkey, table_with_primary_constraint_p_pkey1;
ALTER TABLE table_with_primary_constraint_p ADD UNIQUE (author, title);
INSERT INTO table_with_primary_constraint_p VALUES(1, 1);
INSERT INTO table_with_primary_constraint_p VALUES(2, 2);

-- create external gphdfs table
-- NOTE: We fake the gphdfs protocol here so that it doesn't actually have to be
-- installed.
CREATE OR REPLACE FUNCTION noop() RETURNS integer AS 'select 0' LANGUAGE SQL;
DROP PROTOCOL IF EXISTS gphdfs CASCADE;
CREATE PROTOCOL gphdfs (writefunc=noop, readfunc=noop);

CREATE EXTERNAL TABLE ext_gphdfs (name text)
	LOCATION ('gphdfs://example.com/data/filename.txt')
	FORMAT 'TEXT' (DELIMITER '|');
CREATE EXTERNAL TABLE "ext gphdfs" (name text) -- whitespace in the name
	LOCATION ('gphdfs://example.com/data/filename.txt')
	FORMAT 'TEXT' (DELIMITER '|');

-- create name datatype attributes as the not-first column
DROP TABLE IF EXISTS table_with_name_as_second_column;
CREATE TABLE table_with_name_as_second_column (a int, "first last" name);
INSERT INTO table_with_name_as_second_column VALUES(1, 'George Washington');
INSERT INTO table_with_name_as_second_column VALUES(1, 'Henry Ford');
-- create partition table with name datatype attribute as the not-first column as the partition key
DROP TABLE IF EXISTS partition_table_partitioned_by_name_type;
CREATE TABLE partition_table_partitioned_by_name_type(a int, b name) PARTITION BY RANGE (b) (START('a') END('z'));
-- create table with name datatype attribute as the not-first column as the distribution key
DROP TABLE IF EXISTS table_distributed_by_name_type;
CREATE TABLE table_distributed_by_name_type(a int, b name) DISTRIBUTED BY (b);
INSERT INTO table_distributed_by_name_type VALUES (1,'z'),(2,'x');
-- create table / views with name dataype
CREATE TABLE t1_with_name(a name, b name) DISTRIBUTED RANDOMLY;
INSERT INTO t1_with_name SELECT 'aaa', 'bbb';
CREATE TABLE t2_with_name(a int, b name) DISTRIBUTED RANDOMLY;
INSERT INTO t2_with_name SELECT 1, 'bbb';
CREATE VIEW v2_on_t2_with_name AS SELECT * FROM t2_with_name;
-- multilevel partition table with partitioning keys using name datatype
CREATE TABLE multilevel_part_with_partition_col_name_datatype (trans_id int, country name, amount decimal(9,2), region name)
DISTRIBUTED BY (trans_id)
PARTITION BY LIST (country)
SUBPARTITION BY LIST (region)
SUBPARTITION TEMPLATE
( SUBPARTITION south VALUES ('south'),
    DEFAULT SUBPARTITION other_regions)
    (PARTITION usa VALUES ('usa'),
    DEFAULT PARTITION outlying_country );
-- multilevel partition table with partitioning keys using not using name datatype
CREATE TABLE multilevel_part_with_partition_col_text_datatype (trans_id int, country text, state name, region text)
DISTRIBUTED BY (trans_id)
PARTITION BY LIST (country)
SUBPARTITION BY LIST (region)
SUBPARTITION TEMPLATE
( SUBPARTITION south VALUES ('south'),
    DEFAULT SUBPARTITION other_regions)
    (PARTITION usa VALUES ('usa'),
    DEFAULT PARTITION outlying_country );

-- create tables with tsquery datatype
DROP TABLE IF EXISTS table_with_tsquery_datatype_columns;
CREATE TABLE table_with_tsquery_datatype_columns(a tsquery, b tsquery, c tsquery, d int)
    PARTITION BY RANGE(d) (START(1) END(4) EVERY(1));
INSERT INTO table_with_tsquery_datatype_columns
    VALUES  ('b & c'::tsquery, 'b & c'::tsquery, 'b & c'::tsquery, 1),
            ('e & f'::tsquery, 'e & f'::tsquery, 'e & f'::tsquery, 2),
            ('x & y'::tsquery, 'x & y'::tsquery, 'x & y'::tsquery, 3);


RESET search_path;
