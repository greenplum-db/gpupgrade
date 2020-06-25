-- Copyright (c) 2017-2020 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- Step 1 of 3
-- generates a script to Drop foreign/unique/primary key constraints from partitioned tables.
--    Order is significant. Remove constraints on root partition table before
--    non-partition tables so that we can cascade deleting constraints from
--    child partition tables.
SELECT 'ALTER TABLE ' || nspname || '.' || relname || ' DROP CONSTRAINT ' || conname || ' CASCADE;'
FROM pg_constraint cc
    JOIN
    (SELECT DISTINCT c.oid, n.nspname, c.relname
     FROM pg_catalog.pg_partition p
        JOIN pg_catalog.pg_class c ON (p.parrelid = c.oid)
        JOIN pg_catalog.pg_namespace n ON (n.oid = c.relnamespace)) as sub ON sub.oid=cc.conrelid
WHERE cc.contype IN ('f', 'u', 'p');
