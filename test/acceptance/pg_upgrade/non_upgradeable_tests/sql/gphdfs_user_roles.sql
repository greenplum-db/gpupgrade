-- Copyright (c) 2017-2021 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- GPDB5: Roles created with GPHDFS privileges are non-upgradable, since GPHDFS
-- has been replaced with PXF in higher major versions. Such roles must be
-- dropped before an upgrade.

--------------------------------------------------------------------------------
-- Create and setup non-upgradeable objects
--------------------------------------------------------------------------------
CREATE ROLE gphdfs_role WITH CREATEEXTTABLE(protocol='gphdfs', type='readable') CREATEEXTTABLE(protocol='gphdfs', type='writable');

--------------------------------------------------------------------------------
-- Assert that pg_upgrade --check correctly detects the non-upgradeable objects
--------------------------------------------------------------------------------
!\retcode gpupgrade initialize --source-gphome="${GPHOME_SOURCE}" --target-gphome=${GPHOME_TARGET} --source-master-port=${PGPORT} --disk-free-ratio 0 --automatic;
! cat ~/gpAdminLogs/gpupgrade/pg_upgrade/p-1/gphdfs_user_roles.txt;

--------------------------------------------------------------------------------
-- Workaround to unblock upgrade
--------------------------------------------------------------------------------
DROP ROLE gphdfs_role;
