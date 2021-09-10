-- Copyright (c) 2017-2021 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

WITH distcols as
(
   SELECT
      localoid,
      unnest(attrnums) attnum
   from
      gp_distribution_policy
),
partitionedKeys as
(
   SELECT
      DISTINCT parrelid, unnest(paratts) att_num
   FROM
      pg_catalog.pg_partition p
)
SELECT 'DO $$ BEGIN ALTER TABLE ' || c.oid::pg_catalog.regclass ||
       ' ALTER COLUMN ' || pg_catalog.quote_ident(a.attname) ||
       ' TYPE NAME; EXCEPTION WHEN feature_not_supported THEN PERFORM pg_temp.notsupported(''' || c.oid::pg_catalog.regclass || '''); END $$;'
FROM
   pg_catalog.pg_class c
   JOIN
      pg_catalog.pg_namespace n
      ON c.relnamespace = n.oid
      AND c.relkind = 'r'
      AND n.nspname !~ '^pg_temp_'
      AND n.nspname !~ '^pg_toast_temp_'
      AND n.nspname NOT IN
      (
         'pg_catalog',
         'information_schema',
         'gp_toolkit'
      )
   JOIN
      pg_catalog.pg_attribute a
      ON c.oid = a.attrelid
      AND a.attnum > 1
      AND NOT a.attisdropped
      AND a.atttypid = 'pg_catalog.name'::pg_catalog.regtype
   LEFT JOIN distcols
      ON a.attnum = distcols.attnum
      AND a.attrelid = distcols.localoid
   LEFT JOIN partitionedKeys
      ON a.attnum = partitionedKeys.att_num
      AND a.attrelid = partitionedKeys.parrelid
WHERE
   -- exclude table entries which has a distribution key using name data type
   distcols.attnum is NULL
   -- exclude partition tables entries which has partition columns using name data type
   AND partitionedKeys.parrelid is NULL
   -- exclude child partitions
   AND c.oid NOT IN
       (SELECT DISTINCT parchildrelid
       FROM pg_catalog.pg_partition_rule)
   -- if there is a view dependent on a relation having name column, exclude
   -- the relation from the output
   AND c.oid NOT IN
   (
      SELECT DISTINCT
         d.refobjid
      FROM
         pg_depend d
         JOIN
            pg_rewrite r
            ON r.oid = d.objid
         JOIN
            pg_class v
            ON v.oid = r.ev_class
      WHERE
         relkind = 'v'
         AND d.classid = 'pg_rewrite'::regclass
         AND d.refclassid = 'pg_class'::regclass
         AND d.deptype = 'n'
   )
  AND NOT EXISTS (
    SELECT 1
    FROM
    pg_inherits AS i
    JOIN
    pg_attribute AS a2
    ON i.inhparent = a2.attrelid
    WHERE
    i.inhrelid = a.attrelid
  AND a.attname = a2.attname
    );
