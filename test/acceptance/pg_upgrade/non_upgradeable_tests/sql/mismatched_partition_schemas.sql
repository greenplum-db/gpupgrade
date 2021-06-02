-- Copyright (c) 2017-2021 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- Partitioned tables that have their root and partitions in different schemas
-- are non-upgradeable. Without ATTACH PARTITION, it's very difficult to
-- correctly dump and restore such tables. Thus, these tables are considered
-- non-upgradeable. So, partitions have to be brought under the same
-- namespace before running an upgrade.

CREATE SCHEMA other_schema;

--------------------------------------------------------------------------------
-- Create and setup non-upgradeable objects
--------------------------------------------------------------------------------
CREATE TABLE multischema_partition (a int)
  PARTITION BY RANGE(a) (START(1) END(2) EVERY(1));
ALTER TABLE multischema_partition_1_prt_1 SET SCHEMA other_schema;

CREATE TABLE multischema_subpartition (a int, b int)
  PARTITION BY RANGE(a)
    SUBPARTITION BY RANGE(b)
    SUBPARTITION TEMPLATE (START(1) END(3) EVERY(1), DEFAULT SUBPARTITION other)
  (START(1) END(2) EVERY(1));
ALTER TABLE multischema_subpartition_1_prt_1_2_prt_2 SET SCHEMA other_schema;
ALTER TABLE multischema_subpartition_1_prt_1_2_prt_other SET SCHEMA other_schema;

--------------------------------------------------------------------------------
-- Assert that pg_upgrade --check correctly detects the non-upgradeable objects
--------------------------------------------------------------------------------
!\retcode gpupgrade initialize --source-gphome="${GPHOME_SOURCE}" --target-gphome=${GPHOME_TARGET} --source-master-port=${PGPORT} --disk-free-ratio 0 --automatic;
! /bin/cat ${GPUPGRADE_HOME}/pg_upgrade/seg-1/mismatched_partition_schemas.txt | LC_ALL=C sort -b;

--------------------------------------------------------------------------------
-- Workaround to unblock upgrade
--------------------------------------------------------------------------------
ALTER TABLE other_schema.multischema_partition_1_prt_1 SET SCHEMA public;
ALTER TABLE other_schema.multischema_subpartition_1_prt_1_2_prt_2 SET SCHEMA public;
ALTER TABLE other_schema.multischema_subpartition_1_prt_1_2_prt_other SET SCHEMA public;
