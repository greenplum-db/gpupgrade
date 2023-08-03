-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

--------------------------------------------------------------------------------
-- Validate that the upgradeable objects are functional post-upgrade
--------------------------------------------------------------------------------

-- Note: In the tests below, we do quick sanity checks to verify that
-- the external partitions cannot be inserted into. Writable external
-- tables cannot be exchanged into a partition table. Only readable
-- external tables are allowed so the inserts into the external
-- partitions should always fail. If those inserts don't fail,
-- something is wrong.

SELECT * FROM one_level_partition_table;
INSERT INTO one_level_partition_table VALUES (7,2), (8,2), (9,2);
SELECT * FROM one_level_partition_table;
-- this should fail
INSERT INTO one_level_partition_table VALUES (1,1);

SELECT * FROM two_level_partition_table;
INSERT INTO two_level_partition_table VALUES (7,2,1), (8,2,1), (9,2,1);
SELECT * FROM two_level_partition_table;
-- this should fail
INSERT INTO two_level_partition_table VALUES (1,1,1);

SELECT * FROM three_level_partition_table;
INSERT INTO three_level_partition_table VALUES (7,2,1,'y'), (8,2,1,'y'), (9,2,1,'y');
SELECT * FROM three_level_partition_table;
-- this should fail
INSERT INTO three_level_partition_table VALUES (1,1,1,'y');

SELECT * FROM other_three_level_partition_table;
-- these should fail
INSERT INTO other_three_level_partition_table VALUES (1,1,1,'y');
INSERT INTO other_three_level_partition_table VALUES (1,2,1,'y');
