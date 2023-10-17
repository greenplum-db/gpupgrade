-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

DROP DATABASE IF EXISTS isolation2test;
CREATE ROLE upgradable_objects_role;
CREATE ROLE nonupgradeable_objects_role;
CREATE ROLE migratable_objects_role;

CREATE RESOURCE QUEUE test_queue WITH (
    ACTIVE_STATEMENTS = 2,
    MIN_COST = 1700,
    MAX_COST = 2000,
    COST_OVERCOMMIT = false,
    PRIORITY = MIN,
    MEMORY_LIMIT = '10MB'
);
CREATE RESOURCE GROUP test_group WITH (
    CONCURRENCY = 5,
    CPU_RATE_LIMIT = 5,
    MEMORY_LIMIT = 5,
    MEMORY_SHARED_QUOTA = 5,
    MEMORY_SPILL_RATIO = 5
);
CREATE ROLE resource_group_queue_role resource group test_group resource queue test_queue;
