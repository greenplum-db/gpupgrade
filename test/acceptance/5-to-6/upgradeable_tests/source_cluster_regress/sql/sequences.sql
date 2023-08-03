-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- Test to ensure that sequences can be upgraded.

--------------------------------------------------------------------------------
-- Create and setup upgradeable objects
--------------------------------------------------------------------------------

CREATE SEQUENCE serial_seq START 100;
SELECT nextval('serial_seq');

CREATE TABLE tbl_with_sequence(id INT NOT NULL DEFAULT nextval('serial_seq'), t text);
ALTER SEQUENCE serial_seq OWNED BY tbl_with_sequence.id;

INSERT INTO tbl_with_sequence(t) VALUES('test1');
INSERT INTO tbl_with_sequence(t) VALUES('test2');

SELECT * FROM serial_seq;
