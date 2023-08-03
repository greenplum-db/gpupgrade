-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

--------------------------------------------------------------------------------
-- Validate that the upgradeable objects are functional post-upgrade
--------------------------------------------------------------------------------

SELECT * FROM heap_table_with_check_constraint;
-- this insert should fail
INSERT INTO heap_table_with_check_constraint VALUES (2, 'Jane');

SELECT * FROM partition_table_with_check_constraint;
-- this insert should fail
INSERT INTO partition_table_with_check_constraint VALUES (1, 1, 3);

