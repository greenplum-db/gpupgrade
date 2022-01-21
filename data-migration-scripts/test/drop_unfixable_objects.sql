-- Copyright (c) 2017-2021 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

SET search_path to testschema;

DROP TABLE partition_table_partitioned_by_name_type;
DROP TABLE table_distributed_by_name_type;
DROP TABLE multilevel_part_with_partition_col_name_datatype;

-- Cannot handle cases where we have to change the type of a partition key column
DROP TABLE sales_name;
DROP TABLE sales_tsquery;

RESET search_path;
