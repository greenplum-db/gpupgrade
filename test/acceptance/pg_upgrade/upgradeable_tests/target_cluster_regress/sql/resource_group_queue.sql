-- Copyright (c) 2017-2021 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

--------------------------------------------------------------------------------
-- Validate that the upgradeable objects are functional post-upgrade
--------------------------------------------------------------------------------

-- Validate attributes of resource queues
SELECT rsqname, rsqcountlimit, rsqcostlimit, rsqovercommit,
       rsqignorecostlimit, resname, ressetting
    FROM pg_resqueue r, pg_resqueuecapability c, pg_resourcetype t
    WHERE r.oid=c.resqueueid AND c.restypid=t.restypid
    ORDER BY rsqname;

-- Validate attributes of resource groups
SELECT groupname, concurrency, cpu_rate_limit, memory_limit, memory_shared_quota,
       memory_spill_ratio, memory_spill_ratio, memory_auditor, cpuset
    FROM gp_toolkit.gp_resgroup_config
    ORDER BY groupname;

-- Validate resource queue and group assignment to test_role
SELECT rolname, rsqname, rsgname FROM pg_roles, pg_resgroup, pg_resqueue
    WHERE pg_roles.rolresgroup=pg_resgroup.oid
    AND pg_roles.rolresqueue=pg_resqueue.oid
    AND rolname='test_role';
