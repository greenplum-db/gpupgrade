-- Copyright (c) 2017-2022 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- Detect heterogenous partition tables and CTAS the affected leaf tables
-- The detection query is based on the GPDB pg_upgrade code at:
-- contrib/pg_upgrade/greenplum/check_gp.h
-- check_heterogeneous_partition() in contrib/pg_upgrade/greenplum/check_gp.c
-- We only handle scenario 1 referenced in check_heterogeneous_partition().
-- Detection query used: CHECK_PARTITION_TABLE_DROPPED_COLUMN_REFERENCES

SET client_min_messages TO WARNING;

-- Use CREATE SCHEMA IF NOT EXISTS once it is supported
CREATE OR REPLACE FUNCTION  __gpupgrade_tmp_generator.fix_het()
RETURNS VARCHAR AS
$$
import plpy

swap_sql = ""

res1 = plpy.execute("""
SELECT cp1.childnamespace, cp1.childrelname, rp.parrelname, p3.schemaname, p3.partitionname, p3.partitionrank, p3.partitionposition, p3.parentpartitiontablename
    FROM (
            SELECT p.parrelid, rule.parchildrelid, n.nspname AS childnamespace, c.relname AS childrelname, c.relnatts AS childnatts,
                   sum(CASE WHEN a.attisdropped THEN 1 ELSE 0 END) AS childnumattisdropped
            FROM pg_catalog.pg_partition p
                JOIN pg_catalog.pg_partition_rule rule ON p.oid=rule.paroid AND NOT p.paristemplate
                JOIN pg_catalog.pg_class c ON rule.parchildrelid = c.oid AND NOT c.relhassubclass
                JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
                JOIN pg_catalog.pg_attribute a ON rule.parchildrelid = a.attrelid AND a.attnum > 0
            GROUP BY p.parrelid, rule.parchildrelid, n.nspname, c.relname, c.relnatts
        ) cp1
        JOIN (
            SELECT p.parrelid, min(c.relnatts) AS minchildnatts, max(c.relnatts) AS maxchildnatts
            FROM pg_catalog.pg_partition p
                JOIN pg_catalog.pg_partition_rule rule ON p.oid=rule.paroid AND NOT p.paristemplate
                JOIN pg_catalog.pg_class c ON rule.parchildrelid = c.oid AND NOT c.relhassubclass
            GROUP BY p.parrelid
        ) cp2 ON cp2.parrelid = cp1.parrelid
        JOIN (
            SELECT c.oid, n.nspname AS parnamespace, c.relname AS parrelname, c.relnatts AS parnatts,
                   sum(CASE WHEN a.attisdropped THEN 1 ELSE 0 END) AS parnumattisdropped
            FROM pg_catalog.pg_partition p
                JOIN pg_catalog.pg_class c ON p.parrelid = c.oid AND NOT p.paristemplate AND p.parlevel = 0
                JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
                JOIN pg_catalog.pg_attribute a ON c.oid = a.attrelid AND a.attnum > 0
            GROUP BY c.oid, n.nspname, c.relname, c.relnatts
        ) rp ON rp.oid = cp1.parrelid
        JOIN pg_partitions p3 ON cp1.childrelname = p3.partitiontablename
    WHERE NOT (rp.parnumattisdropped = 0 AND rp.parnatts = cp1.childnatts) AND
          NOT (rp.parnumattisdropped > 0 AND cp2.minchildnatts = cp2.maxchildnatts AND
               (rp.parnatts = cp1.childnatts OR cp1.childnumattisdropped = 0)) AND
          NOT (rp.parnumattisdropped > 0 AND cp2.minchildnatts != cp2.maxchildnatts AND
               cp2.minchildnatts < rp.parnatts AND cp1.childnumattisdropped = 0) AND
          NOT (rp.parnumattisdropped > 0 AND cp2.minchildnatts != cp2.maxchildnatts AND
               cp2.minchildnatts >= rp.parnatts)
    ORDER BY rp.oid, cp1.parchildrelid;
    """)
if res1 is not None:
    for i in res1:
        schemaname = i["schemaname"]
        partitionname = i["partitionname"]
        partitionrank = i["partitionrank"]
        parrelname = i["parrelname"]
        childrelname = i["childrelname"]
        partitionposition = i["partitionposition"]
        parentpartitiontablename = i["parentpartitiontablename"]

        partition_sql = ""
        local_parentpartitiontablename = parentpartitiontablename
        while local_parentpartitiontablename is not None:
            local_vars = plpy.execute("""
                SELECT parentpartitiontablename, partitionrank, partitionname
                FROM pg_partitions
                WHERE partitiontablename = '{local_parentpartitiontablename}' """.format(**locals()))[0]
            local_partitionname = local_vars["partitionname"]
            local_partitionrank = local_vars["partitionrank"]
            if local_partitionname:
                partition_sql = " ALTER PARTITION {local_partitionname} ".format(**locals()) + partition_sql
            elif local_partitionrank:
                partition_sql = " ALTER PARTITION FOR (RANK({local_partitionrank})) ".format(**locals()) + partition_sql
            else:
                plpy.error("Cannot read partition name or rank {local_parentpartitiontablename}".format(**locals()))

            local_parentpartitiontablename = local_vars["parentpartitiontablename"]

        exchange_sql = ""
        if partitionname:
            exchange_sql = " EXCHANGE PARTITION {partitionname} ".format(**locals())
        elif partitionrank:
            exchange_sql = " EXCHANGE PARTITION FOR (RANK({partitionrank})) ".format(**locals())
        else:
            plpy.error("Cannot read partition name or rank {0}".format(parentpartitiontablename))

        swap_sql = swap_sql + """
CREATE TABLE __gpupgrade_tmp_executor.scratch_table AS SELECT * FROM {schemaname}.{childrelname};
ALTER TABLE {schemaname}.{parrelname} {partition_sql} {exchange_sql} WITH TABLE __gpupgrade_tmp_executor.scratch_table;
DROP TABLE __gpupgrade_tmp_executor.scratch_table;
""".format(**locals())

# We create a schema during the executor runtime for the temporary scratch tables.
# This schema has a different name than the generator temp schema to avoid potential double create and/or premature
# drop commands.
if swap_sql is not "":
    swap_sql = """
SET gp_enable_exchange_default_partition = on;
SET optimizer_enable_ctas = off;
CREATE SCHEMA __gpupgrade_tmp_executor;
DROP TABLE IF EXISTS __gpupgrade_tmp_executor.scratch_table;
{0}
DROP SCHEMA __gpupgrade_tmp_executor CASCADE;
RESET gp_enable_exchange_default_partition;
RESET optimizer_enable_ctas;
""".format(swap_sql)
return swap_sql

$$ LANGUAGE plpythonu;
SELECT __gpupgrade_tmp_generator.fix_het();
