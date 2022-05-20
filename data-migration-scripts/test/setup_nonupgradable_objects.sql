-- Copyright (c) 2017-2022 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- CREATE global objects
CREATE DATABASE testdb;
CREATE ROLE gphdfs_user CREATEEXTTABLE(protocol='gphdfs', type='writable') CREATEEXTTABLE(protocol='gphdfs', type='readable');
CREATE ROLE test_role;
