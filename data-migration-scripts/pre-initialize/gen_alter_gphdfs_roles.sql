-- Copyright (c) 2017-2021 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- generates a sql script to drop gphdfs roles in the cluster
SELECT 'ALTER ROLE '|| pg_catalog.quote_ident(rolname) || $$ NOCREATEEXTTABLE(protocol='gphdfs',type='readable'); $$
FROM pg_roles
WHERE rolcreaterexthdfs='t'
UNION ALL
SELECT 'ALTER ROLE ' || pg_catalog.quote_ident(rolname) || $$ NOCREATEEXTTABLE(protocol='gphdfs',type='writable'); $$
FROM pg_roles
WHERE rolcreatewexthdfs='t';
