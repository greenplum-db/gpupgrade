-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

--------------------------------------------------------------------------------
-- Create and setup migratable objects
--------------------------------------------------------------------------------

-- check data
SELECT * from ao_root_partition ORDER BY 1, 2;
SELECT * FROM aoco_root_partition ORDER BY 1, 2;

-- exercise object
INSERT INTO ao_root_partition VALUES(1, 4);
INSERT INTO aoco_root_partition VALUES(1, 4);

-- check data
SELECT * from ao_root_partition ORDER BY 1, 2;
SELECT * FROM aoco_root_partition ORDER BY 1, 2;
