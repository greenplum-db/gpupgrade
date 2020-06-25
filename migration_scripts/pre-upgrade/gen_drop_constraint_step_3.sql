-- Copyright (c) 2017-2020 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- Step 3 or 3
-- generates a script to DROP partition indexes
WITH partitions AS (
    SELECT DISTINCT n.nspname, c.relname
    FROM pg_catalog.pg_partition p
        JOIN pg_catalog.pg_class c ON (p.parrelid = c.oid)
        JOIN pg_catalog.pg_namespace n ON (n.oid = c.relnamespace)
    UNION
    SELECT n.nspname,
           partitiontablename AS relname
    FROM pg_catalog.pg_partitions p
        JOIN pg_catalog.pg_class c ON (p.partitiontablename = c.relname)
        JOIN pg_catalog.pg_namespace n ON (n.oid = c.relnamespace)
)
SELECT 'DROP INDEX '|| nspname ||'.'||indexname||';'
FROM partitions
    JOIN pg_catalog.pg_indexes ON (relname = tablename AND nspname = schemaname);
