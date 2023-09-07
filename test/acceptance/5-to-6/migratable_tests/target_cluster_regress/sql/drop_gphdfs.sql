-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

--------------------------------------------------------------------------------
-- Create and setup migratable objects
--------------------------------------------------------------------------------

-- check gphdfs
SELECT proname FROM pg_proc WHERE proname='noop';
SELECT relname FROM pg_class WHERE relname LIKE '%gphdfs' AND relstorage='x';
SELECT ptcname FROM pg_extprotocol where ptcname='gphdfs';
