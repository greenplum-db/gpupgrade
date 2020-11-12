// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

/*
	The filter command massages the post-upgrade SQL dump by removing known
	differences. Different set of rules are applied for dump from greenplum
	version 5 and 6. In general, the below set of rules are applied on the dump.

	- Line rules are regular expressions that will cause any matching lines to
	be removed immediately.

	- Block rules are regular expressions that cause any matching lines, and any
	preceding comments or blank lines, to be removed.

	- Formatting rules are a set of functions that can format the sql statement tokens
	into a desired format

	filter reads from an input file and writes to stdout. Usage:

		filter -version=5 -inputFile=dump.sql > dump-filtered.sql

	Error handling is basic: any failures result in a log.Fatal() call.
*/
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/greenplum-db/gpupgrade/ci/scripts/filters"
)

var (
	version5 = 5
	version6 = 6
)

type rules struct {
	init func()
}

func newRules(version int) *rules {
	r := &rules{}
	if version == version5 {
		r.init = filters.Init5x
	} else {
		r.init = filters.Init6x
	}

	return r
}

func writeBufAndLine(out io.Writer, buf []string, line string) []string {
	// We want to keep this line. Flush and empty our buffer first.
	if len(buf) > 0 {
		write(out, buf...)
		buf = (buf)[:0]
	}

	write(out, line)

	return buf
}

func write(out io.Writer, lines ...string) {
	for _, line := range lines {
		_, err := fmt.Fprintln(out, line)
		if err != nil {
			log.Fatalf("writing output: %+v", err)
		}
	}
}

func Filter(version int, in io.Reader, out io.Writer) {
	rules := newRules(version)
	rules.init()

	scanner := bufio.NewScanner(in)
	// there are lines in icw regression suite requiring buffer
	// to be atleast 10000000, so keeping it a little higher for now.
	scanner.Buffer(nil, 9800*4024)

	var buf []string // lines buffered for look-ahead

	var formattingContext = filters.NewFormattingContext()

nextline:
	for scanner.Scan() {
		line := scanner.Text()

		formattingContext.Find(filters.Formatters, line)
		if formattingContext.Formatting() {
			formattingContext.AddTokens(line)
			if filters.EndFormatting(line) {
				stmt, err := formattingContext.Format()
				if err != nil {
					log.Fatalf("unexpected error: %#v", err)
				}
				buf = writeBufAndLine(out, buf, stmt)
				formattingContext = filters.NewFormattingContext()
			}
			continue nextline
		}

		// First filter on a line-by-line basis.
		for _, r := range filters.LineRegexes {
			if r.MatchString(line) {
				continue nextline
			}
		}

		if strings.HasPrefix(line, "--") || len(line) == 0 {
			// A comment or an empty line. We only want to output this section
			// if the SQL it's attached to isn't filtered.
			buf = append(buf, line)
			continue nextline
		}

		for _, r := range filters.BlockRegexes {
			if r.MatchString(line) {
				// Discard this line and any buffered comment block.
				buf = buf[:0]
				continue nextline
			}
		}

		for _, replacementFunc := range filters.ReplacementFuncs {
			line = replacementFunc(line)
		}

		buf = writeBufAndLine(out, buf, line)
	}

	if scanner.Err() != nil {
		log.Fatalf("scanning stdin: %+v", scanner.Err())
	}

	// Flush our buffer.
	if len(buf) > 0 {
		write(out, buf...)
	}
}

func main() {
	var (
		version   int
		inputFile string
		argCount  = 2
	)

	flag.IntVar(&version, "version", 0, "identifier specific version of greenplum dump, i.e 5 or 6")
	flag.StringVar(&inputFile, "inputFile", "", "fully qualified input file name containing the dump")
	flag.Parse()

	if flag.NFlag() != argCount {
		fmt.Printf("requires %d arguments, got %d\n", argCount, flag.NFlag())
		flag.Usage()
		os.Exit(1)
	}

	if version != version5 && version != version6 {
		fmt.Printf("permitted -version values are %d and %d. but got %d\n", version5, version6, version)
		os.Exit(1)
	}

	in, err := os.Open(inputFile)
	if err != nil {
		fmt.Print(fmt.Errorf("%s: %w\n", inputFile, err))
		os.Exit(1)
	}

	Filter(version, in, os.Stdout)
}
