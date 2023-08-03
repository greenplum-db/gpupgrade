-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- Test partition tables whose children are in different schemas.

--------------------------------------------------------------------------------
-- Create and setup upgradeable objects
--------------------------------------------------------------------------------

-- Partition table with children partitions where root and child partitions are
-- in different schemas
CREATE SCHEMA schema1;
CREATE SCHEMA schema2;
CREATE TABLE public.different_schema_ptable(a int, b int) PARTITION BY RANGE(a) (START(1) END (4) EVERY(1));

ALTER TABLE public.different_schema_ptable_1_prt_1 SET SCHEMA schema1;
ALTER TABLE public.different_schema_ptable_1_prt_2 SET SCHEMA schema2;

INSERT INTO public.different_schema_ptable VALUES (1, 1);
INSERT INTO public.different_schema_ptable VALUES (2, 1);
INSERT INTO public.different_schema_ptable VALUES (3, 1);

-- check data
SELECT * FROM public.different_schema_ptable ORDER BY 1, 2;
SELECT * FROM schema1.different_schema_ptable_1_prt_1 ORDER BY 1, 2;
SELECT * FROM schema2.different_schema_ptable_1_prt_2 ORDER BY 1, 2;
SELECT * FROM public.different_schema_ptable_1_prt_3 ORDER BY 1, 2;

-- check partition schemas
SELECT nsp.nspname, c.relname
FROM pg_class c
JOIN pg_namespace nsp ON nsp.oid = c.relnamespace
WHERE relname LIKE 'different_schema_ptable%'
ORDER BY relname;



-- Multilevel partition table with children partitions where root and child
-- partitions are in different schemas
CREATE TABLE multilevel_different_schema_ptable (id int, year date, gender char(1))
DISTRIBUTED BY (id, gender, year)
partition BY list (gender)
subpartition BY range (year)
subpartition template (
START (date '2001-01-01'),
START (date '2002-01-01'),
START (date '2003-01-01')
)
(
  partition boys VALUES ('M'),
  partition girls VALUES ('F')
);

ALTER TABLE public.multilevel_different_schema_ptable_1_prt_boys SET SCHEMA schema1;
ALTER TABLE public.multilevel_different_schema_ptable_1_prt_girls_2_prt_1 SET SCHEMA schema1;
ALTER TABLE public.multilevel_different_schema_ptable_1_prt_girls_2_prt_2 SET SCHEMA schema2;

INSERT INTO public.multilevel_different_schema_ptable VALUES (1, date '2001-01-15', 'M');
INSERT INTO public.multilevel_different_schema_ptable VALUES (2, date '2002-02-15', 'M');
INSERT INTO public.multilevel_different_schema_ptable VALUES (3, date '2003-03-15', 'M');
INSERT INTO public.multilevel_different_schema_ptable VALUES (4, date '2001-01-15', 'F');
INSERT INTO public.multilevel_different_schema_ptable VALUES (5, date '2002-02-15', 'F');
INSERT INTO public.multilevel_different_schema_ptable VALUES (6, date '2003-03-15', 'F');

-- check data
SELECT * FROM public.multilevel_different_schema_ptable ORDER BY 1, 2, 3;
SELECT * FROM schema1.multilevel_different_schema_ptable_1_prt_boys ORDER BY 1, 2, 3;
SELECT * FROM public.multilevel_different_schema_ptable_1_prt_boys_2_prt_1 ORDER BY 1, 2, 3;
SELECT * FROM public.multilevel_different_schema_ptable_1_prt_boys_2_prt_2 ORDER BY 1, 2, 3;
SELECT * FROM public.multilevel_different_schema_ptable_1_prt_boys_2_prt_3 ORDER BY 1, 2, 3;
SELECT * FROM public.multilevel_different_schema_ptable_1_prt_girls ORDER BY 1, 2, 3;
SELECT * FROM schema1.multilevel_different_schema_ptable_1_prt_girls_2_prt_1 ORDER BY 1, 2, 3;
SELECT * FROM schema2.multilevel_different_schema_ptable_1_prt_girls_2_prt_2 ORDER BY 1, 2, 3;
SELECT * FROM public.multilevel_different_schema_ptable_1_prt_girls_2_prt_3 ORDER BY 1, 2, 3;

-- check partition schemas
SELECT nsp.nspname, c.relname
FROM pg_class c
JOIN pg_namespace nsp ON nsp.oid = c.relnamespace
WHERE relname LIKE 'multilevel_different_schema_ptable%'
ORDER BY relname;
