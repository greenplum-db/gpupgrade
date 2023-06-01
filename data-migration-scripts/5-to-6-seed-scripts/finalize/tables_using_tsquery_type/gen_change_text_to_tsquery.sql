-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- generates alter statement to modify text datatype to tsquery datatype
WITH partitionedKeys AS
(
   SELECT DISTINCT parrelid, unnest(paratts) att_num
   FROM pg_catalog.pg_partition p
)
SELECT $$ALTER TABLE $$ || pg_catalog.quote_ident(n.nspname) || '.' || pg_catalog.quote_ident(c.relname) ||
       $$ ALTER COLUMN $$ || pg_catalog.quote_ident(a.attname) ||
       $$ TYPE TSQUERY USING $$ || pg_catalog.quote_ident(a.attname) || $$::tsquery;$$
FROM pg_catalog.pg_class c,
     pg_catalog.pg_namespace n,
     pg_catalog.pg_attribute a
     LEFT JOIN partitionedKeys
     ON a.attnum = partitionedKeys.att_num
         AND a.attrelid = partitionedKeys.parrelid
WHERE c.relkind = 'r'
    AND c.oid = a.attrelid
    AND NOT a.attisdropped
    AND a.atttypid = 'pg_catalog.tsquery'::pg_catalog.regtype
    AND c.relnamespace = n.oid
    AND n.nspname NOT LIKE 'pg_temp_%'
    AND n.nspname NOT LIKE 'pg_toast_temp_%'
    AND n.nspname NOT IN ('pg_catalog',
                        'information_schema')
    -- exclude child partitions
    AND c.oid NOT IN
        (SELECT DISTINCT parchildrelid
         FROM pg_catalog.pg_partition_rule)
    -- exclude tables partitioned on a tsquery data type
    AND partitionedKeys.parrelid IS NULL
    -- exclude inherited columns
    AND a.attinhcount = 0;
