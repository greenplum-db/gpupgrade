-- Copyright (c) 2017-2021 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- Portions Copyright © 1996-2020, The PostgreSQL Global Development Group
--
-- Portions Copyright © 1994, The Regents of the University of California
--
-- Permission to use, copy, modify, and distribute this software and its
-- documentation for any purpose, without fee, and without a written
-- agreement is hereby granted, provided that the above copyright notice
-- and this paragraph and the following two paragraphs appear in all copies.
--
-- IN NO EVENT SHALL THE UNIVERSITY OF CALIFORNIA BE LIABLE TO ANY PARTY FOR
-- DIRECT, INDIRECT, SPECIAL, INCIDENTAL, OR CONSEQUENTIAL DAMAGES, INCLUDING
-- LOST PROFITS, ARISING OUT OF THE USE OF THIS SOFTWARE AND ITS DOCUMENTATION,
-- EVEN IF THE UNIVERSITY OF CALIFORNIA HAS BEEN ADVISED OF THE POSSIBILITY
-- OF SUCH DAMAGE.
--
-- THE UNIVERSITY OF CALIFORNIA SPECIFICALLY DISCLAIMS ANY WARRANTIES, INCLUDING,
-- BUT NOT LIMITED TO, THE IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR
-- A PARTICULAR PURPOSE. THE SOFTWARE PROVIDED HEREUNDER IS ON AN "AS IS" BASIS,
-- AND THE UNIVERSITY OF CALIFORNIA HAS NO OBLIGATIONS TO PROVIDE MAINTENANCE,
-- SUPPORT, UPDATES, ENHANCEMENTS, OR MODIFICATIONS.

-- generate ALTER TABLE ALTER COLUMN commands for tables with name datatype attributes
-- The DDL command to alter the name datatype is executed on root partitions and
-- non-partitioned tables. The ALTER command executed on root partitions cascades
-- to child partitions, and thus are excluded here.
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
       ' TYPE VARCHAR(63); EXCEPTION WHEN feature_not_supported THEN PERFORM pg_temp.notsupported(''' || c.oid::pg_catalog.regclass || '''); END $$;'
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
   );
