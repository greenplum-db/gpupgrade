-- Copyright (c) 2017-2021 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- Test to ensure that the base relfilenode for an AO/CO table exists after
-- upgrade. This is necessary because AO/CO tables can exist without a base
-- relfilenode in GPDB5, but not in higher versions. When we encounter a missing
-- AO/CO base relfilenode then upgrade we must create an empty relfilenode for
-- the target cluster.

-- start_matchsubs
-- m/^ERROR:  could not stat file.*/
-- s/^ERROR:  could not stat file.*/ERROR:  could not stat file"/
-- end_matchsubs

--------------------------------------------------------------------------------
-- Create and setup upgradeable objects
--------------------------------------------------------------------------------

-- 1. AO table
CREATE TABLE ao_without_base_relfilenode (a int ,b int) WITH (appendonly=true);
INSERT INTO ao_without_base_relfilenode VALUES (1,1), (2,2), (3,3);

-- Alter the table so that the relfilenode is changed
ALTER TABLE ao_without_base_relfilenode SET DISTRIBUTED RANDOMLY;

-- Delete some records so that we have empty base relfilenodes
DELETE FROM ao_without_base_relfilenode;

INSERT INTO ao_without_base_relfilenode VALUES (1,1), (2,2), (3,3);

-- Vaccum the table so that the empty base relfilenode files are deleted
VACUUM ao_without_base_relfilenode;

-- Check that the base relfilenode does not exist on some segments as desired
SELECT pg_stat_file('base/' || db.oid || '/' || pc.relfilenode) from gp_dist_random('pg_class') pc, gp_dist_random('pg_database') db where pc.relname='ao_without_base_relfilenode' and datname = current_database();

-- 2. CO table
CREATE TABLE aoco_without_base_relfilenode WITH (appendonly=true, orientation=column) AS (
  SELECT GENERATE_SERIES::numeric a, GENERATE_SERIES b FROM GENERATE_SERIES(1, 2)
);

UPDATE aoco_without_base_relfilenode SET b=-b;

VACUUM aoco_without_base_relfilenode;

-- check that the base relfilenode does not exist on some segments
SELECT pg_stat_file('base/' || db.oid || '/' || pc.relfilenode) from gp_dist_random('pg_class') pc, gp_dist_random('pg_database') db where pc.relname='aoco_without_base_relfilenode' and datname = current_database();
