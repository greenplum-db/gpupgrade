// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

/*
	The filter command massages the post-upgrade SQL dump by removing known
	differences. It does this with the following set of rules

	- Line rules are regular expressions that will cause any matching lines to
	be removed immediately.

	- Block rules are regular expressions that cause any matching lines, and any
	preceding comments or blank lines, to be removed.

	- Formatting rules are a set of functions that can format the sql statement tokens
	into a desired format

	filter reads from stdin and writes to stdout. Usage:

		filter < target.sql > target-filtered.sql

	Error handling is basic: any failures result in a log.Fatal() call.
*/
package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strings"

	"github.com/greenplum-db/gpupgrade/ci/scripts/filters"
)

type ReplacementFunc func(line string) string

var replacementFuncs []ReplacementFunc

// function to identify if the line matches a pattern
type shouldFormatFunc func(line string) bool

// function to create a formatted string using the tokens
type formatFunc func(tokens []string) (string, error)

// identifier and corresponding formatting function
type formatter struct {
	shouldFormat shouldFormatFunc
	format       formatFunc
}

// hold the current tokens for the formatting function
type formatContext struct {
	tokens     []string
	formatFunc formatFunc
}

var (
	formatters   []formatter
	lineRegexes  []*regexp.Regexp
	blockRegexes []*regexp.Regexp
)

// is formatting currently in progress
func (f *formatContext) formatting() bool {
	return f.formatFunc != nil
}

func (f *formatContext) addTokens(line string) {
	f.tokens = append(f.tokens, strings.Fields(line)...)
}

func endFormatting(line string) bool {
	return strings.Contains(line, ";")
}

func (f *formatContext) format() (string, error) {
	return f.formatFunc(f.tokens)
}

func newFormattingContext() *formatContext {
	return &formatContext{}
}

func (f *formatContext) find(formatters []formatter, line string) {
	if f.formatFunc != nil {
		return
	}

	for _, x := range formatters {
		if x.shouldFormat(line) {
			f.formatFunc = x.format
			break
		}
	}
}

func init() {
	// linePatterns remove exactly what is matched, on a line-by-line basis.
	linePatterns := []string{
		`ALTER DATABASE .+ SET gp_use_legacy_hashops TO 'on';`,
		// TODO: There may be false positives because of the below
		// pattern, and we might have to do a look ahead to really identify
		// if it can be deleted.
		`START WITH \d`,
	}

	// blockPatterns remove lines that match, AND any comments or whitespace
	// immediately preceding them.
	blockPatterns := []string{
		"CREATE EXTENSION IF NOT EXISTS plpgsql WITH SCHEMA pg_catalog;",
		"COMMENT ON EXTENSION plpgsql IS",
		"COMMENT ON DATABASE postgres IS",
	}

	replacementFuncs = []ReplacementFunc{
		filters.FormatWithClause,
	}

	// patten matching functions and corresponding formatting functions
	formatters = []formatter{
		{shouldFormat: filters.IsViewOrRuleDdl, format: filters.FormatViewOrRuleDdl},
		{shouldFormat: filters.IsTriggerDdl, format: filters.FormatTriggerDdl},
	}

	for _, pattern := range linePatterns {
		lineRegexes = append(lineRegexes, regexp.MustCompile(pattern))
	}
	for _, pattern := range blockPatterns {
		blockRegexes = append(blockRegexes, regexp.MustCompile(pattern))
	}
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

func Filter(in io.Reader, out io.Writer) {
	scanner := bufio.NewScanner(in)
	// there are lines in icw regression suite requiring buffer
	// to be atleast 10000000, so keeping it a little higher for now.
	scanner.Buffer(nil, 9800*4024)

	var buf []string // lines buffered for look-ahead

	var formattingContext = newFormattingContext()

nextline:
	for scanner.Scan() {
		line := scanner.Text()

		formattingContext.find(formatters, line)
		if formattingContext.formatting() {
			formattingContext.addTokens(line)
			if endFormatting(line) {
				stmt, err := formattingContext.format()
				if err != nil {
					log.Fatalf("unexpected error: %#v", err)
				}
				buf = writeBufAndLine(out, buf, stmt)
				formattingContext = newFormattingContext()
			}
			continue nextline
		}

		// First filter on a line-by-line basis.
		for _, r := range lineRegexes {
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

		for _, r := range blockRegexes {
			if r.MatchString(line) {
				// Discard this line and any buffered comment block.
				buf = buf[:0]
				continue nextline
			}
		}

		for _, replacementFunc := range replacementFuncs {
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
	Filter(os.Stdin, os.Stdout)
}
