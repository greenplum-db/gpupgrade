-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- Test to ensure that a table with name columns can be upgraded. This
-- was previously banned but found to be upgradeable.

--------------------------------------------------------------------------------
-- Create and setup upgradeable objects
--------------------------------------------------------------------------------

-- The name columns are strategically placed to test on-disk alignment
-- (padding) before and after upgrade. The int and text column are
-- 4-byte aligned whereas the name column used to be 4-byte aligned
-- but was changed in 5X to have no alignment needed.
CREATE TABLE table_with_name_columns (a int, b name, c text, d name);
CREATE INDEX table_with_name_columns_idx ON table_with_name_columns USING btree(b);

INSERT INTO table_with_name_columns SELECT i, 'bbb' || i, 'ccc' || i, 'ddd' || i FROM generate_series(1,10)i;

-- Do an insert that maxes out the name data type. The name data type
-- is limited to 63 chars which will truncate the z. However, the text
-- data type does not have this limitation and will store the ending z.
INSERT INTO table_with_name_columns VALUES (88, 'aaaaaaaaaabbbbbbbbbbccccccccccddddddddddeeeeeeeeeeffffffffffgggz', 'aaaaaaaaaabbbbbbbbbbccccccccccddddddddddeeeeeeeeeeffffffffffgggz', 'aaaaaaaaaabbbbbbbbbbccccccccccddddddddddeeeeeeeeeeffffffffffgggz');

-- Show the data before upgrade.
SELECT * FROM table_with_name_columns;
