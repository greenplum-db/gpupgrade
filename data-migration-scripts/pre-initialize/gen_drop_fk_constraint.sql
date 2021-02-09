-- Copyright (c) 2017-2021 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- generates a script to drop foreign key constraints from root partition tables.
-- Note: 
-- 1. Foreign key constraints are not allowed to be added on to the child partition.
-- 2. Primary and Foreign Key constraints cannot be dropped on the child partitions directly,
-- also dropping them on the root partition does not cascade the drop of such constraints, as its
-- not tracked in the catalog. So, we don't touch such constraint.
SELECT
   'ALTER TABLE ' || nspname || '.' || relname || ' DROP CONSTRAINT ' || conname || ';'
FROM
   pg_constraint cc
   JOIN
      (
         SELECT DISTINCT
            c.oid,
            n.nspname,
            c.relname
         FROM
            pg_catalog.pg_partition p
            JOIN
               pg_catalog.pg_class c
               ON (p.parrelid = c.oid)
            JOIN
               pg_catalog.pg_namespace n
               ON (n.oid = c.relnamespace)
      )
      as sub
      ON sub.oid = cc.conrelid
WHERE
   cc.contype = 'f';
