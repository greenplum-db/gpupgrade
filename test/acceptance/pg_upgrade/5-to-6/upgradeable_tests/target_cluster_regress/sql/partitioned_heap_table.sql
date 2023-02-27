-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

--------------------------------------------------------------------------------
-- Validate that the upgradeable objects are functional post-upgrade
--------------------------------------------------------------------------------

SELECT * FROM p_basic;

SELECT * FROM p_add_partition_test;

SELECT * FROM p_add_list_partition_test;

SELECT * FROM p_split_partition_test;
INSERT INTO p_split_partition_test SELECT i, i FROM generate_series(6,10)i;
ALTER TABLE p_split_partition_test SPLIT DEFAULT PARTITION START(6) END(10) INTO (PARTITION second_split, PARTITION extra);
SELECT * FROM p_split_partition_test;
SELECT parname, parisdefault FROM pg_partition_rule pr JOIN pg_partition p ON pr.paroid = p.oid WHERE p.parrelid = 'p_split_partition_test'::regclass AND pr.parname != '';

SELECT id, age FROM p_subpart_heap_1_prt_partition_id_2_prt_subpartition_age_first;
SELECT id, age FROM p_subpart_heap_1_prt_partition_id_2_prt_subpartition_age_second;
SELECT id, age FROM p_subpart_heap;

SELECT b, c FROM dropped_column WHERE a=10;

SELECT b, c FROM root_has_dropped_column WHERE a=10;

SELECT c, d FROM dropped_and_added_column WHERE a=10;

SELECT c.relname, pg_catalog.pg_get_userbyid(c.relowner) as owner
FROM pg_class c
WHERE relname like 'p_alter_owner%';
