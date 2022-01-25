-- Copyright (c) 2017-2021 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- If a root partition contains a foreign key constraint, pg_dump will create
-- the DDL in the below format:
--
-- CREATE TABLE public.pt ( a integer )
--	DISTRIBUTED BY (a) PARTITION BY RANGE(a) ( START (1) END (3) EVERY (2) WITH
--	(tablename='pt_1_prt_1', appendonly=false ) );
--
-- ALTER TABLE ONLY public.pt ADD CONSTRAINT pt_fkey FOREIGN KEY (a)
-- REFERENCES public.mfk(a);
--
-- When the ALTER statement is executed on the target cluster, the below error
-- is observed:
--
-- ERROR: can't add a constraint to "pt"; it is a partitioned table or part thereof
--
-- Thus, foreign key constraints on root partitions are non-upgradeable and must
-- be dropped before upgrading the cluster.

--------------------------------------------------------------------------------
-- Create and setup non-upgradeable objects
--------------------------------------------------------------------------------
CREATE TABLE mfk(a int unique);
INSERT INTO mfk SELECT i FROM generate_series(1,2)i;
CREATE TABLE pt(a int references mfk(a)) PARTITION BY RANGE(a) (START(1) END(3) EVERY(2));
INSERT INTO pt SELECT i FROM generate_series(1,2)i;

CREATE TABLE pt_another(a int references mfk(a)) PARTITION BY RANGE(a) (START(1) END(3) EVERY(2));
INSERT INTO pt_another SELECT i FROM generate_series(1,2)i;

CREATE TABLE non_pt(a int references mfk(a));
INSERT INTO non_pt SELECT i FROM generate_series(1,2)i;

--------------------------------------------------------------------------------
-- Assert that pg_upgrade --check correctly detects the non-upgradeable objects
--------------------------------------------------------------------------------
!\retcode gpupgrade initialize --source-gphome="${GPHOME_SOURCE}" --target-gphome=${GPHOME_TARGET} --source-master-port=${PGPORT} --disk-free-ratio 0 --automatic;
! cat ~/gpAdminLogs/gpupgrade/pg_upgrade/p-1/foreign_key_constraints.txt | LC_ALL=C sort -b;

--------------------------------------------------------------------------------
-- Workaround to unblock upgrade
--------------------------------------------------------------------------------
ALTER TABLE public.pt DROP CONSTRAINT pt_fkey;
ALTER TABLE public.pt_another DROP CONSTRAINT pt_another_fkey;
