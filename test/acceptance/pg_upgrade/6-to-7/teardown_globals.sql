-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

DROP DATABASE IF EXISTS isolation2test;
DROP ROLE upgradable_objects_role;
DROP ROLE nonupgradeable_objects_role;
DROP ROLE migratable_objects_role;
DROP ROLE resource_group_queue_role;

DROP RESOURCE GROUP test_group;
DROP RESOURCE QUEUE test_queue;
