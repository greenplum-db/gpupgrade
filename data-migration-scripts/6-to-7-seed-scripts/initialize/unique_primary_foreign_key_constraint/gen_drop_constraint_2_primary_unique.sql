-- Copyright (c) 2017-2022 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- Generate a script to drop unique/primary key constraints.

-- For upgrades from 5X we need to set gp_enable_drop_key_constraint_child_partition so we can drop unique and primary
-- key constraints directly from leaf partitions, in addition to dropping them from the root. Normally, dropping
-- constraints directly from leaves is banned by GPDB - and dropping constraints from the root suffices. However,
-- dropping such constraints from the root doesn't always drop the constraint from the leaves (if an unnamed constraint
-- was added to the partition hierarchy with ALTER TABLE ADD CONSTRAINT at the root level).

-- For upgrades from 6X onwards, it is sufficient to drop the constraint from the root level.

SELECT 'SET gp_enable_drop_key_constraint_child_partition=on;';
SELECT
   'ALTER TABLE ' || pg_catalog.quote_ident(n.nspname) || '.' || pg_catalog.quote_ident(cc.relname) || ' DROP CONSTRAINT ' || pg_catalog.quote_ident(conname) || ' CASCADE;'
FROM
   pg_constraint con
   JOIN
      pg_depend dep
      ON (refclassid, classid, objsubid) =
      (
         'pg_constraint'::regclass,
         'pg_class'::regclass,
         0
      )
      AND refobjid = con.oid
      AND deptype = 'i'
      AND contype IN
      (
         'u',
         'p'
      )
   JOIN
      pg_class c
      ON objid = c.oid
      AND relkind = 'i'
   JOIN
      pg_class cc
      ON cc.oid = con.conrelid
   JOIN
      pg_namespace n
      ON (n.oid = cc.relnamespace);
SELECT 'SET gp_enable_drop_key_constraint_child_partition=off;';
