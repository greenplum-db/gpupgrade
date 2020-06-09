// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"testing"
)

func TestFilter(t *testing.T) {
	t.Run("it writes stdin to stdout when nothing is filtered", func(t *testing.T) {
		var in, out bytes.Buffer

		expected := "hello\n"
		in.WriteString(expected)

		Filter(&in, &out)
		if out.String() != expected {
			t.Errorf("wrote %q want %q", out.String(), expected)
		}
	})

	t.Run("filters out legacy hashops settings", func(t *testing.T) {
		var in, out bytes.Buffer

		in.WriteString(`
GRANT ALL ON DATABASE template1 TO gpadmin;
GRANT CONNECT ON DATABASE template1 TO PUBLIC;
ALTER DATABASE template1 SET gp_use_legacy_hashops TO 'on';
SET allow_system_table_mods = true;
CREATE DATABASE test WITH TEMPLATE = template0 OWNER = gpadmin;
RESET allow_system_table_mods;
ALTER DATABASE test SET gp_use_legacy_hashops TO 'on';
`)

		Filter(&in, &out)

		expected := `
GRANT ALL ON DATABASE template1 TO gpadmin;
GRANT CONNECT ON DATABASE template1 TO PUBLIC;
SET allow_system_table_mods = true;
CREATE DATABASE test WITH TEMPLATE = template0 OWNER = gpadmin;
RESET allow_system_table_mods;
`
		if out.String() != expected {
			t.Errorf("wrote %q want %q", out.String(), expected)
			t.Logf("actual (expanded): %s", out.String())
			t.Logf("expected (expanded): %s", expected)
		}
	})

	t.Run("filters out empty and commented lines attached to filtered SQL", func(t *testing.T) {
		var in, out bytes.Buffer

		in.WriteString(`
GRANT ALL ON DATABASE template1 TO gpadmin;

--
-- Name: plpgsql; Type: EXTENSION; Schema: -; Owner:
--

CREATE EXTENSION IF NOT EXISTS plpgsql WITH SCHEMA pg_catalog;


RESET allow_system_table_mods;
`)

		Filter(&in, &out)

		expected := `
GRANT ALL ON DATABASE template1 TO gpadmin;


RESET allow_system_table_mods;
`
		if out.String() != expected {
			t.Errorf("wrote %q want %q", out.String(), expected)
			t.Logf("actual (expanded): %s", out.String())
			t.Logf("expected (expanded): %s", expected)
		}
	})

	t.Run("keeps trailing comment blocks", func(t *testing.T) {
		var in, out bytes.Buffer

		in.WriteString(`

--
-- Greenplum Database database dump complete
--

--
-- PostgreSQL database cluster dump complete
--

`)

		Filter(&in, &out)

		expected := `

--
-- Greenplum Database database dump complete
--

--
-- PostgreSQL database cluster dump complete
--

`
		if out.String() != expected {
			t.Errorf("wrote %q want %q", out.String(), expected)
			t.Logf("actual (expanded): %s", out.String())
			t.Logf("expected (expanded): %s", expected)
		}
	})

	t.Run("for retail demo data remove quotes for partition table rel options", func(t *testing.T) {
		var in, out bytes.Buffer

		in.WriteString(`
START ('2005-12-01 00:00:00'::timestamp without time zone) END ('2006-01-01 00:00:00'::timestamp without time zone) EVERY ('1 mon'::interval) WITH (tablename='order_lineitems_1_prt_2', appendonly='true', compresstype=quicklz, orientation='column' )
`)

		expected := `
START ('2005-12-01 00:00:00'::timestamp without time zone) END ('2006-01-01 00:00:00'::timestamp without time zone) EVERY ('1 mon'::interval) WITH (tablename='order_lineitems_1_prt_2', appendonly=true, compresstype=quicklz, orientation=column )
`

		Filter(&in, &out)

		if out.String() != expected {
			t.Errorf("wrote %q want %q", out.String(), expected)
			t.Logf("actual (expanded): %s", out.String())
			t.Logf("expected (expanded): %s", expected)
		}
	})

	t.Run("for retail demo data do not remove quotes for regular table rel options", func(t *testing.T) {
		var in, out bytes.Buffer

		expected := "WITH (appendonly='true', compresstype=quicklz, orientation='column'\n"
		in.WriteString(expected)

		Filter(&in, &out)

		if out.String() != expected {
			t.Errorf("wrote %q want %q", out.String(), expected)
			t.Logf("actual (expanded):   %s", out.String())
			t.Logf("expected (expanded): %s", expected)
		}
	})

	t.Run("formats the view body to single line", func(t *testing.T) {
		var in, out bytes.Buffer
		in.WriteString(`--
-- Name: t3; Type: VIEW; Schema: public; Owner: xxxx
--
CREATE VIEW public.t3 AS
 SELECT t1.s2,
    foo.s2_xform
   FROM (public.t1
     JOIN ( SELECT t2.s2,
            COALESCE((avg(t2.r) - 0.020000), (0)::numeric) AS s2_xform
           FROM public.t2
          GROUP BY t2.s2) foo ON ((t1.s2 = foo.s2)));`)

		expected := `--
-- Name: t3; Type: VIEW; Schema: public; Owner: xxxx
--
CREATE VIEW public.t3 AS
SELECT t1.s2, foo.s2_xform FROM (public.t1 JOIN (SELECT t2.s2, COALESCE((avg(t2.r) - 0.020000), (0)::numeric) AS s2_xform FROM public.t2 GROUP BY t2.s2) foo ON ((t1.s2 = foo.s2)));
`

		Filter(&in, &out)

		if out.String() != expected {
			t.Errorf("wrote %q want %q", out.String(), expected)
		}
	})

	t.Run("formats the rule ddl to single line", func(t *testing.T) {
		var in, out bytes.Buffer
		in.WriteString(`--
-- Name: oid_consistency_bar2 two; Type: RULE; Schema: public; Owner: gpadmin
--
CREATE RULE two AS
    ON INSERT TO public.oid_consistency_bar2 DO INSTEAD  INSERT INTO public.oid_consistency_foo2 (a)
  VALUES (1);`)

		expected := `--
-- Name: oid_consistency_bar2 two; Type: RULE; Schema: public; Owner: gpadmin
--
CREATE RULE two AS ON INSERT TO public.oid_consistency_bar2 DO INSTEAD INSERT INTO public.oid_consistency_foo2 (a) VALUES (1);
`

		Filter(&in, &out)

		if out.String() != expected {
			t.Errorf("wrote %q want %q", out.String(), expected)
		}
	})
}
