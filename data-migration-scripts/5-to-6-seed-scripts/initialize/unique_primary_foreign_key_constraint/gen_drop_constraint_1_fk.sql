-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- generates a script to drop foreign key constraints.
-- Foreign key constraints have to be dropped before primary/unique constraints to make sure that
-- we can successfully drop the dependee constraints.
-- Note that we primary and unique constraints need to be created before
-- foreign key constraints such that they can be properly referenced referenced.
-- Thus, we place them in the same subdirectory.
SELECT
   'ALTER TABLE ' || pg_catalog.quote_ident(nspname) || '.' || pg_catalog.quote_ident(relname) || ' DROP CONSTRAINT ' || pg_catalog.quote_ident(conname) || ';'
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
