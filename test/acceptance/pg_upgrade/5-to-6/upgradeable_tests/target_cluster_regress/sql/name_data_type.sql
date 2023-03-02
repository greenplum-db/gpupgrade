-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

--------------------------------------------------------------------------------
-- Validate that the upgradeable objects are functional post-upgrade
--------------------------------------------------------------------------------

-- Show the data after upgrade.
SELECT * FROM table_with_name_columns;

-- Show that the btree index still works after upgrade.
SET enable_seqscan = off;
EXPLAIN (COSTS off) SELECT * FROM table_with_name_columns WHERE b = 'bbb8';
SELECT * FROM table_with_name_columns WHERE b = 'bbb8';
SET enable_seqscan = on;

-- Make sure the table is still usable after upgrade.
DELETE FROM table_with_name_columns WHERE a < 5;
UPDATE table_with_name_columns SET b = 'bbb888' WHERE a = 8;
INSERT INTO table_with_name_columns VALUES (888, 'aaaaaaaaaabbbbbbbbbbccccccccccddddddddddeeeeeeeeeeffffffffffgggz', 'aaaaaaaaaabbbbbbbbbbccccccccccddddddddddeeeeeeeeeeffffffffffgggz', 'aaaaaaaaaabbbbbbbbbbccccccccccddddddddddeeeeeeeeeeffffffffffgggz');
SELECT * FROM table_with_name_columns;
