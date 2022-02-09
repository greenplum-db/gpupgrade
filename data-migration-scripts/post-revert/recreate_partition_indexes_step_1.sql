-- Copyright (c) 2017-2021 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- generates SQL statement to create indexes on root partition tables
-- that don't correspond to unique or primary key constraints

-- cte to get all the unique and primary key constraints
WITH root_partitions (relid) AS
(
   SELECT DISTINCT
      parrelid
   FROM
      pg_partition
)
,
root_constraints AS
(
   SELECT
      conname,
      c.relname conrel,
      n.nspname relschema,
      cc.relname rel
   FROM
      pg_constraint con
      JOIN
         pg_depend dep
         ON (dep.refclassid, dep.classid, dep.objsubid) =
         (
            'pg_constraint'::regclass,
            'pg_class'::regclass,
            0
         )
         AND dep.refobjid = con.oid
         AND dep.deptype = 'i'
         AND con.contype IN
         (
            'u',
            'p'
         )
      JOIN
         pg_class c
         ON dep.objid = c.oid
         AND c.relkind = 'i'
      JOIN
         root_partitions
         ON con.conrelid = root_partitions.relid
      JOIN
         pg_class cc
         ON cc.oid = con.conrelid
      JOIN
         pg_namespace n
         ON (n.oid = cc.relnamespace)
)
,
indexes AS
(
   SELECT
      n.nspname AS schemaname,
      c.relname AS tablename,
      i.relname AS indexname,
      t.spcname AS tablespace,
      pg_get_indexdef(i.oid) AS indexdef
   FROM
      pg_index x
      JOIN
         root_partitions rp
         on rp.relid = x.indrelid
      JOIN
         pg_class c
         ON c.oid = x.indrelid
      JOIN
         pg_class i
         ON i.oid = x.indexrelid
      LEFT JOIN
         pg_namespace n
         ON n.oid = c.relnamespace
      LEFT JOIN
         pg_tablespace t
         ON t.oid = i.reltablespace
   WHERE
      c.relkind = 'r'::"char"
      AND i.relkind = 'i'::"char"
)
SELECT
$$SET SEARCH_PATH=$$ || schemaname || $$; $$ || indexdef || $$;$$
FROM
   indexes
WHERE
   (
      indexname,
      schemaname,
      tablename
   )
   NOT IN
   (
      SELECT
         conrel,
         relschema,
         rel
      FROM
         root_constraints
   )
;
