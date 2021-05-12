-- Copyright (c) 2017-2021 VMware, Inc. or its affiliates
-- SPDX-License-Identifier: Apache-2.0

-- Test to ensure that PL functions can be upgraded.

--------------------------------------------------------------------------------
-- Create and setup upgradeable objects
--------------------------------------------------------------------------------
CREATE LANGUAGE plpythonu;

CREATE FUNCTION someimmutablepythonfunction(foo integer) RETURNS integer IMMUTABLE STRICT AS $$
return 42 + foo
$$ LANGUAGE plpythonu;

CREATE FUNCTION someimmutablepsqlfunction(foo integer) /* in func */
RETURNS integer
LANGUAGE plpgsql IMMUTABLE STRICT AS
$$
BEGIN /* in func */
	return 42 + foo; /* in func */
END /* in func */
$$;
