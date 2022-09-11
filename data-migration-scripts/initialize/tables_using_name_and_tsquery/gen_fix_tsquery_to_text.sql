-- Copyright (c) 2017-2022 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- Columns having an index on a tsquery column can't be altered, so generate a drop statement for them
WITH distcols AS
         (
             SELECT localoid, unnest(attrnums) attnum
             FROM gp_distribution_policy
         ),
     partitionedKeys AS
         (
             SELECT DISTINCT parrelid, unnest(paratts) att_num
             FROM pg_catalog.pg_partition p
         )
SELECT $$DROP INDEX IF EXISTS $$ || pg_catalog.quote_ident(n.nspname) || '.' || pg_catalog.quote_ident(xc.relname) || ';'
FROM
    pg_catalog.pg_class c
    JOIN pg_catalog.pg_namespace n
ON c.relnamespace = n.oid
    JOIN pg_index x ON c.oid = x.indrelid
    JOIN pg_class xc ON x.indexrelid = xc.oid
WHERE
    EXISTS (
    SELECT 1 FROM pg_catalog.pg_attribute a
    LEFT JOIN distcols
    ON a.attnum = distcols.attnum
        AND a.attrelid = distcols.localoid
    LEFT JOIN partitionedKeys
    ON a.attnum = partitionedKeys.att_num
        AND a.attrelid = partitionedKeys.parrelid
    WHERE a.attrelid = c.oid
        AND a.attnum = ANY (x.indkey)
        AND a.atttypid = 'pg_catalog.tsquery'::pg_catalog.regtype
        AND NOT a.attisdropped
-- exclude table entries which has a distribution key using name data type
        AND distcols.attnum IS NULL
-- exclude partition tables entries which has partition columns using name data type
        AND partitionedKeys.parrelid IS NULL
-- exclude inherited columns
        AND a.attinhcount = 0
    )
    AND c.relkind = 'r'
    AND xc.relkind = 'i'
    AND n.nspname NOT LIKE 'pg_temp_%'
    AND n.nspname NOT LIKE 'pg_toast_temp_%'
    AND n.nspname NOT IN ('pg_catalog', 'information_schema')
    AND c.oid NOT IN
        (SELECT DISTINCT parchildrelid
        FROM pg_catalog.pg_partition_rule);

-- generates alter statement to modify tsquery datatype to text datatype
WITH distcols AS
         (
             SELECT localoid, unnest(attrnums) attnum
             FROM gp_distribution_policy
         ),
     partitionedKeys AS
         (
             SELECT DISTINCT parrelid, unnest(paratts) att_num
             FROM pg_catalog.pg_partition p
         )
SELECT 'DO $$ BEGIN ALTER TABLE ' ||
       pg_catalog.quote_ident(n.nspname) || '.' || pg_catalog.quote_ident(c.relname) ||
       ' ALTER COLUMN ' || pg_catalog.quote_ident(a.attname) ||
       ' TYPE VARCHAR(63); EXCEPTION WHEN feature_not_supported THEN PERFORM pg_temp.notsupported(''' ||
       c.oid::pg_catalog.regclass || '''); END $$;'
FROM pg_catalog.pg_class c,
     pg_catalog.pg_namespace n,
     pg_catalog.pg_attribute a
         LEFT JOIN distcols
                   ON a.attnum = distcols.attnum
                       AND a.attrelid = distcols.localoid
         LEFT JOIN partitionedKeys
                   ON a.attnum = partitionedKeys.att_num
                       AND a.attrelid = partitionedKeys.parrelid
WHERE
  -- exclude table entries which has a distribution key using tsquery data type
    distcols.attnum IS NULL
  -- exclude partition tables entries which has partition columns using tsquery data type
  AND partitionedKeys.parrelid IS NULL
  -- exclude inherited columns
  AND a.attinhcount = 0
  AND c.relkind = 'r'
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
;
