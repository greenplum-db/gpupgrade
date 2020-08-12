-- Copyright (c) 2017-2020 VMware, Inc. or its affiliates
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
SELECT 'DO $$ BEGIN ALTER TABLE ' || c.oid::pg_catalog.regclass ||
       ' ALTER COLUMN ' || pg_catalog.quote_ident(a.attname) ||
       ' TYPE VARCHAR(63); EXCEPTION WHEN feature_not_supported THEN PERFORM pg_temp.notsupported(''' || c.oid::pg_catalog.regclass || '''); END $$;'
FROM pg_catalog.pg_class c,
     pg_catalog.pg_namespace n,
     pg_catalog.pg_attribute a
WHERE c.oid = a.attrelid
  AND a.attnum > 1
  AND NOT a.attisdropped
  AND a.atttypid = 'pg_catalog.name'::pg_catalog.regtype
  AND c.relnamespace = n.oid
  AND -- exclude possible orphaned temp tables
        n.nspname !~ '^pg_temp_'
  AND n.nspname !~ '^pg_toast_temp_'
  AND n.nspname NOT IN ('pg_catalog',
                        'information_schema',
                        'gp_toolkit')
  AND c.oid NOT IN
      (SELECT DISTINCT parchildrelid
       FROM pg_catalog.pg_partition_rule);
