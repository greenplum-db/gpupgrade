-- Copyright (c) 2017-2023 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- Test to ensure that encodings like compresstype, blocksize, and
-- compresslevel on user-defined types are preserved during an upgrade.

--------------------------------------------------------------------------------
-- Create and setup upgradeable objects
--------------------------------------------------------------------------------

CREATE TYPE psuedo_type;
-- This should fail. Cannot set encodings of psuedo types.
ALTER TYPE public.psuedo_type SET DEFAULT ENCODING (compresstype=zlib, blocksize=8192, compresslevel=1);

CREATE TYPE composite_type AS (
    length double precision,
    width double precision,
    depth double precision
);
-- This should fail. Cannot set encodings of composite types.
ALTER TYPE public.composite_type SET DEFAULT ENCODING (compresstype=zlib, blocksize=8192, compresslevel=1);

-- base type
CREATE TYPE int_rle_type;

CREATE FUNCTION int_rle_type_in(cstring)
 RETURNS int_rle_type
 AS 'int4in'
 LANGUAGE internal IMMUTABLE STRICT;

CREATE FUNCTION int_rle_type_out(int_rle_type)
 RETURNS cstring
 AS 'int4out'
 LANGUAGE internal IMMUTABLE STRICT;

CREATE TYPE int_rle_type(
 input = int_rle_type_in,
 output = int_rle_type_out,
 internallength = 4,
 default = 55,
 passedbyvalue,
 compresstype = rle_type,
 blocksize = 8192,
 compresslevel = 1);

CREATE TYPE char_zlib_type;

CREATE FUNCTION char_zlib_type_in(cstring)
 RETURNS char_zlib_type
 AS 'charin'
 LANGUAGE internal IMMUTABLE STRICT;

CREATE FUNCTION char_zlib_type_out(char_zlib_type)
 RETURNS cstring
 AS 'charout'
 LANGUAGE internal IMMUTABLE STRICT;

CREATE TYPE char_zlib_type(
 input = char_zlib_type_in,
 output = char_zlib_type_out,
 internallength = 4,
 default = 'y',
 passedbyvalue,
 compresstype = zlib,
 blocksize = 16384,
 compresslevel = 2);

CREATE DOMAIN us_zip_code AS TEXT CHECK
       ( VALUE ~ '^\d{5}$' OR VALUE ~ '^\d{5}-\d{4}$' );
ALTER TYPE public.us_zip_code SET DEFAULT ENCODING (compresstype=zlib, blocksize=32768, compresslevel=3);

CREATE TYPE mood AS ENUM ('sad', 'ok', 'happy');
ALTER TYPE public.mood SET DEFAULT ENCODING (compresstype=zlib, blocksize=65536, compresslevel=4);
-- 5X does not set dependent array type to the same encoding when using ALTER TYPE
ALTER TYPE public._mood SET DEFAULT ENCODING (compresstype=zlib, blocksize=65536, compresslevel=4);

CREATE TABLE aoco_custom_encoded_types(a int, b int_rle_type, c char_zlib_type, d us_zip_code, e mood) WITH (appendonly=true, orientation=column);
INSERT INTO aoco_custom_encoded_types VALUES (0, '1', 'a', '11111', 'sad');
INSERT INTO aoco_custom_encoded_types VALUES (1, '20', 'b', '22222', 'ok');
INSERT INTO aoco_custom_encoded_types VALUES (2, '123', 'c', '33333-3333', 'happy');

SELECT t.typname, te.typoptions FROM pg_type_encoding te LEFT JOIN pg_type t ON (t.oid=te.typid);
