-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

DROP DATABASE IF EXISTS isolation2test;
CREATE ROLE testrole;
CREATE ROLE test_role1;

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
CREATE ROLE test_role resource group test_group resource queue test_queue;

CREATE FUNCTION drop_gphdfs() RETURNS VOID AS $$
DECLARE
    rolerow RECORD;
BEGIN
    RAISE NOTICE 'Dropping gphdfs users...';
    FOR rolerow IN SELECT * FROM pg_catalog.pg_roles LOOP
            EXECUTE 'alter role '
                        || quote_ident(rolerow.rolname) || ' '
                || 'NOCREATEEXTTABLE(protocol=''gphdfs'',type=''readable'')';
            EXECUTE 'alter role '
                        || quote_ident(rolerow.rolname) || ' '
                || 'NOCREATEEXTTABLE(protocol=''gphdfs'',type=''writable'')';
            RAISE NOTICE 'dropping gphdfs from role % ...', quote_ident(rolerow.rolname);
        END LOOP;
END;
$$ LANGUAGE plpgsql;

SELECT drop_gphdfs();
DROP FUNCTION drop_gphdfs();
DROP PROTOCOL IF EXISTS gphdfs CASCADE;
