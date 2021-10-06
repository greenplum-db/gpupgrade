-- Copyright (c) 2017-2021 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

SELECT
    $$ALTER TABLE $$ || pg_catalog.quote_ident(nspname) || $$.$$ || pg_catalog.quote_ident(relname) ||
    $$ ADD CONSTRAINT $$ || pg_catalog.quote_ident(conname) || $$ $$ ||
    pg_catalog.pg_get_constraintdef(cc.oid, false)  || $$;$$
FROM
    pg_constraint cc
        JOIN
    (
        SELECT DISTINCT
            c.oid,
            n.nspname,
            c.relname
        FROM
            pg_catalog.pg_partition p
                JOIN
            pg_catalog.pg_class c
            ON (p.parrelid = c.oid)
                JOIN
            pg_catalog.pg_namespace n
            ON (n.oid = c.relnamespace)
    )
        as sub
    ON sub.oid = cc.conrelid
WHERE
        cc.contype = 'f';
