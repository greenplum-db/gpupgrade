/*
	The filter command massages the post-upgrade SQL dump by removing known
	differences. It does this with two sets of rules -- lines and blocks.

	- Line rules are regular expressions that will cause any matching lines to
	be removed immediately.

	- Block rules are regular expressions that cause any matching lines, and any
	preceding comments or blank lines, to be removed.

	The main complication here comes from the block rules, which require us to
	use a lookahead buffer.

	filter reads from stdin and writes to stdout. Usage:

		filter < new.sql > new-filtered.sql

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
)

type replacer struct {
	Regex       *regexp.Regexp
	Replacement string
}

func (t *replacer) replace(line string) string {
	return t.Regex.ReplaceAllString(line, t.Replacement)
}

var lineRegexes []*regexp.Regexp
var blockRegexes []*regexp.Regexp
var replacements []*replacer

func init() {
	// linePatterns remove exactly what is matched, on a line-by-line basis.
	linePatterns := []string{
		"ALTER DATABASE .+ SET gp_use_legacy_hashops TO 'on';",
	}

	// blockPatterns remove lines that match, AND any comments or whitespace
	// immediately preceding them.
	blockPatterns := []string{
		"CREATE EXTENSION IF NOT EXISTS plpgsql WITH SCHEMA pg_catalog;",
		"COMMENT ON EXTENSION plpgsql IS",
		"COMMENT ON DATABASE postgres IS",
	}

	// replacementPatterns is a map of regex substitutions.
	replacementPatterns := map[string]string{
		`WITH \(tablename='(.+)', appendonly='true', compresstype=(.+), orientation='column' \)`: `WITH (tablename='${1}', appendonly=true, compresstype=${2}, orientation=column )`,
	}

	for _, pattern := range linePatterns {
		lineRegexes = append(lineRegexes, regexp.MustCompile(pattern))
	}
	for _, pattern := range blockPatterns {
		blockRegexes = append(blockRegexes, regexp.MustCompile(pattern))
	}
	for regex, replacement := range replacementPatterns {
		replacements = append(replacements, &replacer{
			Regex:       regexp.MustCompile(regex),
			Replacement: replacement,
		})
	}
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

	var buf []string // lines buffered for look-ahead

nextline:
	for scanner.Scan() {
		line := scanner.Text()

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

		for _, r := range replacements {
			line = r.replace(line)
		}

		// We want to keep this line. Flush and empty our buffer first.
		if len(buf) > 0 {
			write(out, buf...)
			buf = buf[:0]
		}

		write(out, line)
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
