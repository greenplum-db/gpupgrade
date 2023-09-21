-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- Test to ensure that resource groups and resource queues can be upgraded.

--------------------------------------------------------------------------------
-- Create and setup upgradeable objects
--------------------------------------------------------------------------------

-- Ensure overridden attributes of the default queue and default groups are upgraded
ALTER RESOURCE QUEUE pg_default WITH (ACTIVE_STATEMENTS=2, MIN_COST=1700, MAX_COST=2000, COST_OVERCOMMIT=false, PRIORITY=MIN, MEMORY_LIMIT ='10MB');

ALTER RESOURCE GROUP admin_group SET CONCURRENCY 5;
ALTER RESOURCE GROUP admin_group SET CPU_RATE_LIMIT 5;
ALTER RESOURCE GROUP admin_group SET MEMORY_LIMIT 5;
ALTER RESOURCE GROUP admin_group SET MEMORY_SHARED_QUOTA 5;
ALTER RESOURCE GROUP admin_group SET MEMORY_SPILL_RATIO 5;

ALTER RESOURCE GROUP default_group SET MEMORY_LIMIT 5;
ALTER RESOURCE GROUP default_group SET CPU_RATE_LIMIT 5;
ALTER RESOURCE GROUP default_group SET MEMORY_LIMIT 5;
ALTER RESOURCE GROUP default_group SET MEMORY_SHARED_QUOTA 5;
ALTER RESOURCE GROUP default_group SET MEMORY_SPILL_RATIO 5;

-- Validate attributes of resource queues
SELECT rsqname, rsqcountlimit, rsqcostlimit, rsqovercommit,
       rsqignorecostlimit, resname, ressetting
    FROM pg_resqueue r, pg_resqueuecapability c, pg_resourcetype t
    WHERE r.oid=c.resqueueid AND c.restypid=t.restypid
    ORDER BY rsqname;

-- Validate attributes of resource groups
SELECT groupname, concurrency, proposed_concurrency, cpu_rate_limit,
       memory_limit, proposed_memory_limit, memory_shared_quota,
       proposed_memory_shared_quota, memory_spill_ratio, proposed_memory_spill_ratio
    FROM gp_toolkit.gp_resgroup_config
    ORDER BY groupname;

-- Validate resource queue and group assignment to test_role
SELECT rolname, rsqname, rsgname FROM pg_roles, pg_resgroup, pg_resqueue
    WHERE pg_roles.rolresgroup=pg_resgroup.oid
    AND pg_roles.rolresqueue=pg_resqueue.oid
    AND rolname='test_role';
