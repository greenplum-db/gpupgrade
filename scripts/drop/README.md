Copyright (c) 2017-2020 VMware, Inc. or its affiliates

SPDX-License-Identifier: Apache-2.0

Background
==========

Iniitialize runs a "pg_upgrade --check", which can find various
database objects that cannot be upgraded.  

NOTE: right now, these scripts are to be run against a gpdb-5 database.

Introduction
============

The scripts here generate SQL scripts that drop non-upgradable objects. 

The scripts do not drop the objects directly, as a user might want
to inspect the objects first.

Notes
======

These generating scripts can be run in any order BUT THE RESULTING DROP SCRIPTS
SHOULD BE RUN IN THIS ORDER:

* script_to_drop_constraints_step_1.sql
* script_to_drop_constraints_step_2.sql
* script_to_drop_constraints_step_3.sql

BEFORE DROPPING
===============

Run the script_to_recreate_partition_indexes.sql first and save off the
results:

    psql -d <database> -t -f script_to_recreate_partition_indexes.sql

Running
=======

This will only output the tuple results(not the number of rows or column headers)

    psql -d <database> -t -f script_to_drop_constraints_step_1.sql
