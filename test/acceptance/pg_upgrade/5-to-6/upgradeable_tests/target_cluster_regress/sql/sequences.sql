-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

--------------------------------------------------------------------------------
-- Validate that the upgradeable objects are functional post-upgrade
--------------------------------------------------------------------------------

SELECT * FROM serial_seq;
SELECT * FROM tbl_with_sequence;

SELECT nextval('serial_seq');
INSERT INTO tbl_with_sequence(t) VALUES ('test3');

SELECT * FROM serial_seq;
SELECT * FROM tbl_with_sequence;
