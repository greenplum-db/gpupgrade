-- Copyright (c) 2017-2020 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- generates a sql script to drop external tables in the cluster
SELECT 'DROP EXTERNAL TABLE ' || d.objid::regclass || ';'
FROM pg_catalog.pg_depend d
       JOIN pg_catalog.pg_exttable x ON ( d.objid = x.reloid )
       JOIN pg_catalog.pg_extprotocol p ON ( p.oid = d.refobjid )
       JOIN pg_catalog.pg_class c ON ( c.oid = d.objid )
WHERE d.refclassid = 'pg_extprotocol'::regclass
    AND p.ptcname = 'gphdfs';
