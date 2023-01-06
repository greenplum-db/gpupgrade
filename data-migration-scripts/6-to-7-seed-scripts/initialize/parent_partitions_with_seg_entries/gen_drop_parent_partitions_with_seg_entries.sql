-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- Generates a script to truncate non-empty segrels for AO and AOCO parent
-- partitions.

SELECT 'SET allow_system_table_mods TO TRUE;';

SELECT 'DELETE FROM ' || segrelid::regclass || ';'
FROM pg_appendonly a JOIN pg_class c ON a.relid = c.oid
WHERE c.oid IN (SELECT parrelid FROM pg_partition
                 UNION SELECT parchildrelid
                 FROM pg_partition_rule)
      AND c.relhassubclass = true
      AND a.relid IS NOT NULL
      AND a.segrelid IS NOT NULL
ORDER BY 1;

SELECT 'RESET allow_system_table_mods;';
