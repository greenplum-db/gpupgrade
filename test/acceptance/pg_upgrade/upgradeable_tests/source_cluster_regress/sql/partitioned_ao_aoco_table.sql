-- Copyright (c) 2017-2022 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- Miscellaneous tests checking upgradeability of AO and AOCO partition tables.

--------------------------------------------------------------------------------
-- Create and setup upgradeable objects
--------------------------------------------------------------------------------

-- 1. AO partition table with multiple segfiles and deleted tuples.
CREATE TABLE p_ao_table_with_multiple_segfiles (id int, name text) WITH (appendonly=true) DISTRIBUTED BY (id)
    PARTITION BY RANGE (id)
        SUBPARTITION BY LIST (name)
            SUBPARTITION TEMPLATE (
            SUBPARTITION jane VALUES ('Jane'),
            SUBPARTITION john VALUES ('John'),
            DEFAULT SUBPARTITION other_names )
        (START (1) END (2) EVERY (1),
        DEFAULT PARTITION other_ids);

-- Use multiple sessions to create multiple segfiles
1:BEGIN;
1:INSERT INTO p_ao_table_with_multiple_segfiles VALUES (1, 'Jane');
1:INSERT INTO p_ao_table_with_multiple_segfiles VALUES (2, 'Jane');

2:BEGIN;
2:INSERT INTO p_ao_table_with_multiple_segfiles VALUES (1, 'Jane');
2:INSERT INTO p_ao_table_with_multiple_segfiles VALUES (2, 'Jane');
2:INSERT INTO p_ao_table_with_multiple_segfiles VALUES (4, 'Andy');

1:END;
2:END;

UPDATE p_ao_table_with_multiple_segfiles SET name='Carolyn' WHERE name='Andy';
INSERT INTO p_ao_table_with_multiple_segfiles VALUES (5, 'Bob');
DELETE FROM p_ao_table_with_multiple_segfiles WHERE id=5;

-- 2. AOCO partition table with multiple segfiles and deleted tuples.
CREATE TABLE p_aoco_table_with_multiple_segfiles (id int, name text) WITH (appendonly=true, orientation=column) DISTRIBUTED BY (id)
PARTITION BY RANGE (id)
    SUBPARTITION BY LIST (name)
        SUBPARTITION TEMPLATE (
         SUBPARTITION jane VALUES ('Jane'),
          SUBPARTITION john VALUES ('John'),
           DEFAULT SUBPARTITION other_names )
(START (1) END (2) EVERY (1),
    DEFAULT PARTITION other_ids);

-- Use multiple sessions to create multiple segfiles
1:BEGIN;
1:INSERT INTO p_aoco_table_with_multiple_segfiles VALUES (1, 'Jane');
1:INSERT INTO p_aoco_table_with_multiple_segfiles VALUES (2, 'Jane');

2:BEGIN;
2:INSERT INTO p_aoco_table_with_multiple_segfiles VALUES (1, 'Jane');
2:INSERT INTO p_aoco_table_with_multiple_segfiles VALUES (2, 'Jane');
2:INSERT INTO p_aoco_table_with_multiple_segfiles VALUES (4, 'Andy');

1:END;
2:END;

UPDATE p_aoco_table_with_multiple_segfiles SET name='Carolyn' WHERE name='Andy';
INSERT INTO p_aoco_table_with_multiple_segfiles VALUES (5, 'Bob');
DELETE FROM p_aoco_table_with_multiple_segfiles WHERE id=5;
