-- Copyright (c) 2017-2022 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- Check to ensure that we don't accidentally truncate CLOG during segment
-- upgrade and end up with unfrozen user tuples referring to truncated CLOG.
-- Scanning such tuples post upgrade would result in clog lookup ERRORs such as:
--   SELECT count(*) FROM foo;
--   ERROR:  could not access status of transaction 693  (seg0 slice1 192.168.0.148:50434 pid=2191113)
--   DETAIL:  Could not open file "pg_clog/0000": No such file or directory.

--------------------------------------------------------------------------------
-- Create and setup upgradeable objects
--------------------------------------------------------------------------------

CREATE TABLE foo(i int);

-- Burn through 1 CLOG segment on seg 0, inserting some tuples along the way.
-- These tuples would contain xmins that refer to CLOG that should not be
-- truncated. We ensure that all tuples inserted end up in seg 0.

!\retcode (/bin/bash -c "source ${GPHOME_SOURCE}/greenplum_path.sh && ${GPHOME_SOURCE}/bin/gpconfig -c debug_burn_xids -v on --skipvalidation");
!\retcode (/bin/bash -c "source ${GPHOME_SOURCE}/greenplum_path.sh && ${GPHOME_SOURCE}/bin/gpstop -au");
!\retcode echo "INSERT INTO foo VALUES(1);" > /tmp/clog_preservation.sql;

!\retcode $GPHOME_SOURCE/bin/pgbench -n -f /tmp/clog_preservation.sql -c 8 -t 512 isolation2test;

!\retcode (/bin/bash -c "source ${GPHOME_SOURCE}/greenplum_path.sh && ${GPHOME_SOURCE}/bin/gpconfig -r debug_burn_xids --skipvalidation");
!\retcode (/bin/bash -c "source ${GPHOME_SOURCE}/greenplum_path.sh && ${GPHOME_SOURCE}/bin/gpstop -au");
!\retcode rm /tmp/clog_preservation.sql;

-- NOTE: Do not scan the table here, as it will prevent CLOG lookups when we
-- scan the table post upgrade (doing so sets visibility hint bits on the tuples).
