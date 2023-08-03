-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

SELECT t.typname, te.typoptions FROM pg_type_encoding te LEFT JOIN pg_type t ON (t.oid=te.typid);

INSERT INTO aoco_custom_encoded_types VALUES (3, '444', 'd', '44444-4444', 'happy');

SELECT * FROM aoco_custom_encoded_types;
