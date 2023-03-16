-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

SET search_path to testschema;

-- Cannot handle cases where we have to change the type of a partition key column
DROP TABLE sales_tsquery;

RESET search_path;
