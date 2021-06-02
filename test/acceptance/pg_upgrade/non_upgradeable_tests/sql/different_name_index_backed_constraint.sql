-- Copyright (c) 2017-2021 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- For unique or primary key constraints, the index name is auto generated, and
-- if the default index name is already taken by other objects, an incremental
-- number is appended to the index name. This means that pg_upgrade cannot
-- upgrade a cluster containing indexes of such type, they must be handled
-- manually before/after the upgrade. Although, the issue exists only with such
-- indexes, we wholesale ban upgrading of unique or primary key constraints.
-- Such, constraints must be dropped before the upgrade, and can be recreated
-- after the upgrade.

--------------------------------------------------------------------------------
-- Create and setup non-upgradeable objects
--------------------------------------------------------------------------------
CREATE TYPE table_with_unique_constraint_author_key AS (dummy int);
CREATE TYPE table_with_unique_constraint_author_key1 AS (dummy int);
-- If constraint is named then it leads to table which cannot be recreated from
-- pg_dump due to mismatch between backed index name and constraint name.
CREATE TABLE table_with_unique_constraint (author int, title int, CONSTRAINT table_with_unique_constraint_uniq_au_ti UNIQUE (author, title)) DISTRIBUTED BY (author);
DROP TYPE table_with_unique_constraint_author_key, table_with_unique_constraint_author_key1;
ALTER TABLE table_with_unique_constraint ADD PRIMARY KEY (author, title);
INSERT INTO table_with_unique_constraint VALUES(1, 1);
INSERT INTO table_with_unique_constraint VALUES(2, 2);

CREATE TYPE table_with_primary_constraint_pkey AS (dummy int);
CREATE TYPE table_with_primary_constraint_pkey1 AS (dummy int);
CREATE TABLE table_with_primary_constraint (author int, title int, CONSTRAINT table_with_primary_constraint_au_ti PRIMARY KEY (author, title)) DISTRIBUTED BY (author);
DROP TYPE table_with_primary_constraint_pkey, table_with_primary_constraint_pkey1;
ALTER TABLE table_with_primary_constraint ADD UNIQUE (author, title);
INSERT INTO table_with_primary_constraint VALUES(1, 1);
INSERT INTO table_with_primary_constraint VALUES(2, 2);

--------------------------------------------------------------------------------
-- Assert that pg_upgrade --check correctly detects the non-upgradeable objects
--------------------------------------------------------------------------------
!\retcode gpupgrade initialize --source-gphome="${GPHOME_SOURCE}" --target-gphome=${GPHOME_TARGET} --source-master-port=${PGPORT} --disk-free-ratio 0 --automatic;
! cat ${GPUPGRADE_HOME}/pg_upgrade/seg-1/unique_primary_key_constraint.txt | sort -b -d;

--------------------------------------------------------------------------------
-- Workaround to unblock upgrade
--------------------------------------------------------------------------------
ALTER TABLE table_with_unique_constraint DROP CONSTRAINT table_with_unique_constraint_uniq_au_ti;
ALTER TABLE table_with_primary_constraint DROP CONSTRAINT table_with_primary_constraint_au_ti;
