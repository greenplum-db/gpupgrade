-- Copyright (c) 2017-2020 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- generates a sql script to restore gphdfs roles in the cluster
SELECT 'ALTER ROLE '|| rolname || $$ CREATEEXTTABLE(protocol='gphdfs',type='readable'); $$
FROM pg_roles
WHERE rolcreaterexthdfs='t'
UNION ALL
SELECT 'ALTER ROLE ' || rolname || $$ CREATEEXTTABLE(protocol='gphdfs',type='writable'); $$
FROM pg_roles
WHERE rolcreatewexthdfs='t';
