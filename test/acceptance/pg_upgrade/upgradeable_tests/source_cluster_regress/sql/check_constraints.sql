-- Copyright (c) 2017-2022 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- Test to ensure that tables with simple check constraints can be upgraded.

--------------------------------------------------------------------------------
-- Create and setup upgradeable objects
--------------------------------------------------------------------------------
create table users_with_check_constraints (
    id int, 
    name text check (id>=1 and id<2)
);

insert into users_with_check_constraints values (1, 'Joe');
insert into users_with_check_constraints values (2, 'Jane');
