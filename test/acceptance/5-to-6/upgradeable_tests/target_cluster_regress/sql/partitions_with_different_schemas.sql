-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- Test partition tables whose children are in different schemas.

--------------------------------------------------------------------------------
-- Create and setup upgradeable objects
--------------------------------------------------------------------------------

-- check data integrity after upgrade
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

-- test table insert
INSERT INTO public.different_schema_ptable VALUES (1, 2);
INSERT INTO public.different_schema_ptable VALUES (2, 2);
INSERT INTO public.different_schema_ptable VALUES (3, 2);

-- check data after insert
SELECT * FROM public.different_schema_ptable ORDER BY 1, 2;
SELECT * FROM schema1.different_schema_ptable_1_prt_1 ORDER BY 1, 2;
SELECT * FROM schema2.different_schema_ptable_1_prt_2 ORDER BY 1, 2;
SELECT * FROM public.different_schema_ptable_1_prt_3 ORDER BY 1, 2;



-- check data integrity after upgrade
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

-- test table insert
INSERT INTO public.multilevel_different_schema_ptable VALUES (7, date '2001-01-15', 'M');
INSERT INTO public.multilevel_different_schema_ptable VALUES (8, date '2002-02-15', 'M');
INSERT INTO public.multilevel_different_schema_ptable VALUES (9, date '2003-03-15', 'M');
INSERT INTO public.multilevel_different_schema_ptable VALUES (10, date '2001-01-15', 'F');
INSERT INTO public.multilevel_different_schema_ptable VALUES (11, date '2002-02-15', 'F');
INSERT INTO public.multilevel_different_schema_ptable VALUES (12, date '2003-03-15', 'F');

-- check data after insert
SELECT * FROM public.multilevel_different_schema_ptable ORDER BY 1, 2, 3;
SELECT * FROM schema1.multilevel_different_schema_ptable_1_prt_boys ORDER BY 1, 2, 3;
SELECT * FROM public.multilevel_different_schema_ptable_1_prt_boys_2_prt_1 ORDER BY 1, 2, 3;
SELECT * FROM public.multilevel_different_schema_ptable_1_prt_boys_2_prt_2 ORDER BY 1, 2, 3;
SELECT * FROM public.multilevel_different_schema_ptable_1_prt_boys_2_prt_3 ORDER BY 1, 2, 3;
SELECT * FROM public.multilevel_different_schema_ptable_1_prt_girls ORDER BY 1, 2, 3;
SELECT * FROM schema1.multilevel_different_schema_ptable_1_prt_girls_2_prt_1 ORDER BY 1, 2, 3;
SELECT * FROM schema2.multilevel_different_schema_ptable_1_prt_girls_2_prt_2 ORDER BY 1, 2, 3;
SELECT * FROM public.multilevel_different_schema_ptable_1_prt_girls_2_prt_3 ORDER BY 1, 2, 3;
