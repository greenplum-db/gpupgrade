-- Copyright (c) 2017-2021 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- generates alter statement to modify tsquery datatype to text datatype
SELECT $$ALTER TABLE $$|| n.nspname || '.' || c.relname || $$ ALTER COLUMN $$ || a.attname || $$ TYPE TEXT; $$
FROM pg_catalog.pg_class c,
     pg_catalog.pg_namespace n,
     pg_catalog.pg_attribute a
WHERE c.relkind = 'r'
  AND c.oid = a.attrelid
  AND NOT a.attisdropped
  AND a.atttypid = 'pg_catalog.tsquery'::pg_catalog.regtype
  AND c.relnamespace = n.oid
  AND n.nspname NOT LIKE 'pg_temp_%'
  AND n.nspname NOT LIKE 'pg_toast_temp_%'
  AND n.nspname NOT IN ('pg_catalog',
                        'information_schema')
  AND c.oid NOT IN
      (SELECT DISTINCT parchildrelid
       FROM pg_catalog.pg_partition_rule);
