-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

--------------------------------------------------------------------------------
-- Create and setup migratable objects
--------------------------------------------------------------------------------

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
