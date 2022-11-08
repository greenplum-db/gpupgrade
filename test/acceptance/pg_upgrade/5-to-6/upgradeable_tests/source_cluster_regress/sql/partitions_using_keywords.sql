-- Copyright (c) 2017-2022 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- Ensure that partition names having keywords (reserved, non-reserved and
-- unclassified) can be upgraded by quoting them using the quote_all_identifiers
-- GUC.

--------------------------------------------------------------------------------
-- Create and setup upgradeable objects
--------------------------------------------------------------------------------

--
-- Create a partitioned table using reserved (ie: "window"), non-reserved
-- (ie: "current"), and unclassified (ie: "allocate") keywords for partition names.
-- For a comprehensive list of keywords:
-- https://www.postgresql.org/docs/8.3/sql-keywords-appendix.html
CREATE TABLE t_quote_test (a int, b int, c int, d int, e text)
    DISTRIBUTED BY (a)
    PARTITION BY RANGE (b)
        SUBPARTITION BY RANGE (c)
            SUBPARTITION TEMPLATE (
            START (1) END (2) EVERY (1),
            DEFAULT SUBPARTITION "current" )
        SUBPARTITION BY LIST (e)
            SUBPARTITION TEMPLATE (
            SUBPARTITION "allocate" VALUES ('val1'),
            SUBPARTITION "window" VALUES ('val2'),
            DEFAULT SUBPARTITION dsp )
        ( START (2002) END (2003) EVERY (1),
        DEFAULT PARTITION dp );
