-- Copyright (c) 2017-2021 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- Large objects are not supported GPDB6 onwards. They must be dropped before
-- an upgrade.

--------------------------------------------------------------------------------
-- Create and setup non-upgradeable objects
--------------------------------------------------------------------------------
SELECT lo_create(1);
SELECT count(*) FROM pg_largeobject;

--------------------------------------------------------------------------------
-- Assert that pg_upgrade --check correctly detects the non-upgradeable objects
--------------------------------------------------------------------------------
!\retcode gpupgrade initialize --source-gphome="${GPHOME_SOURCE}" --target-gphome=${GPHOME_TARGET} --source-master-port=${PGPORT} --disk-free-ratio 0 --automatic;
! cat ~/gpAdminLogs/gpupgrade/pg_upgrade/p-1/pg_largeobject.txt;

--------------------------------------------------------------------------------
-- Workaround to unblock upgrade
--------------------------------------------------------------------------------
SELECT lo_unlink(loid) FROM pg_largeobject;
