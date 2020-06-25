-- Copyright (c) 2017-2020 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- Step 2 of 3
-- Generate a script to to DROP unique/primary key constraints
SELECT 'ALTER TABLE '||n.nspname||'.'||cc.relname||' DROP CONSTRAINT '||conname||' CASCADE;'
FROM pg_constraint con
    JOIN pg_depend dep ON (refclassid, classid, objsubid) = ('pg_constraint'::regclass, 'pg_class'::regclass, 0) AND
                          refobjid = con.oid AND
                          deptype = 'i' AND
                          contype IN ('u', 'p')
    JOIN pg_class c ON objid = c.oid AND relkind = 'i'
    JOIN pg_class cc ON cc.oid = con.conrelid
    JOIN pg_namespace n ON (n.oid = cc.relnamespace)
WHERE conname <> c.relname;
