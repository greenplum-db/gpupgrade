-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

--------------------------------------------------------------------------------
-- Validate that the partitioned polymorphic tables work as expected
-- after upgrade.
--------------------------------------------------------------------------------

-- Show what the storage types of each partition are after upgrade
SELECT relname, relstorage FROM pg_class WHERE relname SIMILAR TO 'poly_(list|range)_partition_with_(heap|aoco)_root%' ORDER BY relname;

-- Run some simple DELETEs, UPDATEs, and INSERTs to see if things
-- still work after upgrade
SELECT * FROM poly_range_partition_with_heap_root;
DELETE FROM poly_range_partition_with_heap_root WHERE b%2 = 0 AND b > 1;
UPDATE poly_range_partition_with_heap_root SET b = b - 1 WHERE b > 1;
INSERT INTO poly_range_partition_with_heap_root SELECT 100 + i, i FROM generate_series(2, 9)i;
SELECT * FROM poly_range_partition_with_heap_root;

SELECT * FROM poly_range_partition_with_aoco_root;
DELETE FROM poly_range_partition_with_aoco_root WHERE b%2 = 0 AND b > 1;
UPDATE poly_range_partition_with_aoco_root SET b = b - 1 WHERE b > 1;
INSERT INTO poly_range_partition_with_aoco_root SELECT 100 + i, i FROM generate_series(2, 9)i;
SELECT * FROM poly_range_partition_with_aoco_root;

SELECT * FROM poly_list_partition_with_heap_root;
DELETE FROM poly_list_partition_with_heap_root WHERE b%2 = 0 AND b > 1;
UPDATE poly_list_partition_with_heap_root SET b = b - 1 WHERE b > 1;
INSERT INTO poly_list_partition_with_heap_root SELECT 100 + i, i FROM generate_series(2, 9)i;
SELECT * FROM poly_list_partition_with_heap_root;

SELECT * FROM poly_list_partition_with_aoco_root;
DELETE FROM poly_list_partition_with_aoco_root WHERE b%2 = 0 AND b > 1;
UPDATE poly_list_partition_with_aoco_root SET b = b - 1 WHERE b > 1;
INSERT INTO poly_list_partition_with_aoco_root SELECT 100 + i, i FROM generate_series(2, 9)i;
SELECT * FROM poly_list_partition_with_aoco_root;
