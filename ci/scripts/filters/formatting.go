// Copyright (c) 2017-2020 VMware, Inc. or its affiliates
// SPDX-License-Identifier: Apache-2.0

package filters

import (
	"regexp"
	"strings"
)

var (
	Formatters       []formatter
	LineRegexes      []*regexp.Regexp
	BlockRegexes     []*regexp.Regexp
	ReplacementFuncs []ReplacementFunc
)

// function to identify if the line matches a pattern
type shouldFormatFunc func(buf []string, line string) bool

// function to create a formatted string using the tokens
type formatFunc func(tokens []string) (string, error)

// identifier and corresponding formatting function
type formatter struct {
	shouldFormat shouldFormatFunc
	format       formatFunc
}

type ReplacementFunc func(line string) string

// hold the current tokens for the formatting function
type formatContext struct {
	tokens     []string
	formatFunc formatFunc
}

func NewFormattingContext() *formatContext {
	return &formatContext{}
}

// is formatting currently in progress
func (f *formatContext) Formatting() bool {
	return f.formatFunc != nil
}

func (f *formatContext) AddTokens(line string) {
	f.tokens = append(f.tokens, strings.Fields(line)...)
}

func EndFormatting(line string) bool {
	return strings.Contains(line, ";")
}

func (f *formatContext) Format(buf []string) (string, error) {
	return f.formatFunc(f.tokens)
}

func (f *formatContext) Find(formatters []formatter, buf []string, line string) {
	if f.formatFunc != nil {
		return
	}

	for _, x := range formatters {
		if x.shouldFormat(buf, line) {
			f.formatFunc = x.format
			break
		}
	}
}
