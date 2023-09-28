-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

--------------------------------------------------------------------------------
-- Create and setup migratable objects
--------------------------------------------------------------------------------

-- This is a workaround because the setup to drop this protocol in template1
-- does not persist when when running in the CI for unknown reasons. It is in
-- an ignore because it is not needed locally and produces different output.
-- start_ignore
DROP PROTOCOL IF EXISTS gphdfs CASCADE;
-- end_ignore

-- create external gphdfs table fake the gphdfs protocol so that it doesn't
-- actually have to be installed
CREATE FUNCTION noop() RETURNS integer AS 'select 0' LANGUAGE SQL;
CREATE PROTOCOL gphdfs (writefunc=noop, readfunc=noop);

CREATE EXTERNAL TABLE ext_gphdfs (name text)
	LOCATION ('gphdfs://example.com/data/filename.txt')
	FORMAT 'TEXT' (DELIMITER '|');
CREATE EXTERNAL TABLE "ext gphdfs" (name text)
	LOCATION ('gphdfs://example.com/data/filename.txt')
	FORMAT 'TEXT' (DELIMITER '|');

-- check gphdfs
SELECT proname FROM pg_proc WHERE proname='noop';
SELECT relname FROM pg_class WHERE relname LIKE '%gphdfs' AND relstorage='x';
SELECT ptcname FROM pg_extprotocol where ptcname='gphdfs';
