-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

--------------------------------------------------------------------------------
-- Validate that the upgradeable objects are functional post-upgrade
--------------------------------------------------------------------------------
INSERT INTO alter_dist_key_for_ao_partition_table VALUES (1, 'a', 1);
VACUUM FREEZE alter_dist_key_for_ao_partition_table;
SELECT * FROM alter_dist_key_for_ao_partition_table;


INSERT INTO alter_dist_key_for_aoco_partition_table VALUES (1, 'a', 1);
VACUUM FREEZE alter_dist_key_for_aoco_partition_table;
SELECT * FROM alter_dist_key_for_aoco_partition_table;


INSERT INTO ao_insert_empty_row VALUES (1, 'a', 1);
VACUUM FREEZE ao_insert_empty_row;
SELECT * FROM ao_insert_empty_row;


INSERT INTO aoco_insert_empty_row VALUES (1, 'a', 1);
VACUUM FREEZE aoco_insert_empty_row;
SELECT * FROM aoco_insert_empty_row;
