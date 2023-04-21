-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- Test to ensure that tables with special characters can be upgraded.

--------------------------------------------------------------------------------
-- Create and setup upgradeable objects
--------------------------------------------------------------------------------

CREATE TABLE "foo$"(i int);
CREATE TABLE "$$foo$"(i int);
CREATE TABLE "my_table_@pple"(i int);
CREATE TABLE "users!@#$%^&*()_+`-={}|[];':\""<>,.?/"(i int);
CREATE TABLE "Café_Latté"(i int);
CREATE TABLE "data_2021-09-25"(i int);
CREATE TABLE "Sales@2023"(i int);
CREATE TABLE "table_(parenthesis)"(i int);
CREATE TABLE "Ελληνικά_τραπέζια"(i int);
CREATE TABLE "table_with_underscores and spaces"(i int);
CREATE TABLE "table_with_ö_umlaut"(i int);
CREATE TABLE "table_with_की_hindi_characters"(i int);
CREATE TABLE "学生表"(i text);

INSERT INTO "foo$" (i) VALUES (1);
INSERT INTO "$$foo$" (i) VALUES (2);
INSERT INTO "my_table_@pple" (i) VALUES (3);
INSERT INTO "users!@#$%^&*()_+`-={}|[];':\""<>,.?/" (i) VALUES (4);
INSERT INTO "Café_Latté" (i) VALUES (5);
INSERT INTO "data_2021-09-25" (i) VALUES (6);
INSERT INTO "Sales@2023" (i) VALUES (7);
INSERT INTO "table_(parenthesis)" (i) VALUES (8);
INSERT INTO "Ελληνικά_τραπέζια" (i) VALUES (9);
INSERT INTO "table_with_underscores and spaces" (i) VALUES (10);
INSERT INTO "table_with_ö_umlaut" (i) VALUES (11);
INSERT INTO "table_with_की_hindi_characters" (i) VALUES (12);
INSERT INTO "学生表" (i) VALUES ('张三');

SELECT * FROM "foo$";
SELECT * FROM "$$foo$";
SELECT * FROM "my_table_@pple";
SELECT * FROM "users!@#$%^&*()_+`-={}|[];':\""<>,.?/";
SELECT * FROM "Café_Latté";
SELECT * FROM "data_2021-09-25";
SELECT * FROM "Sales@2023";
SELECT * FROM "table_(parenthesis)";
SELECT * FROM "Ελληνικά_τραπέζια";
SELECT * FROM "table_with_underscores and spaces";
SELECT * FROM "table_with_ö_umlaut";
SELECT * FROM "table_with_की_hindi_characters";
SELECT * FROM "学生表";
