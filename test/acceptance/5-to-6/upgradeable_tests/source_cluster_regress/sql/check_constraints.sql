-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- Test to ensure that tables with simple check constraints can be upgraded.

--------------------------------------------------------------------------------
-- Create and setup upgradeable objects
--------------------------------------------------------------------------------
CREATE TABLE heap_table_with_check_constraint (
    id INT,
    name text CHECK (id>=1 AND id<2)
);

INSERT INTO heap_table_with_check_constraint VALUES (1, 'Joe');
-- this insert should fail
INSERT INTO heap_table_with_check_constraint VALUES (2, 'Jane');

CREATE TABLE partition_table_with_check_constraint (
    a INT CONSTRAINT a_check CHECK (a+b>c),
    b INT,
    c INT) DISTRIBUTED BY (a)
    PARTITION BY RANGE(b)
        (PARTITION part START(0) END(4242));

INSERT INTO partition_table_with_check_constraint SELECT i, i, i FROM generate_series(1, 10) i;
-- this insert should fail
INSERT INTO partition_table_with_check_constraint VALUES (1, 1, 3);
