// Licensed to the Apache Software Foundation (ASF) under one
// or more contributor license agreements.  See the NOTICE file
// distributed with this work for additional information
// regarding copyright ownership.  The ASF licenses this file
// to you under the Apache License, Version 2.0 (the
// "License"); you may not use this file except in compliance
// with the License.  You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package header

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/apache/skywalking-eyes/pkg/comments"

	"github.com/stretchr/testify/require"
)

const oldNotice = "CONFIDENTIAL\nCopyright 2018 Example\nAll rights reserved."
const newNotice = "Copyright [year] Example\nLicensed under Apache-2.0."
const validNotice = `Copyright [0-9]{4}(?:[-, ]+[0-9]{2,4})* Example
Licensed under Apache-2\.0\.`
const oldNoticePattern = `CONFIDENTIAL
Copyright (?P<year>[0-9]{4}(?:[-, ]+[0-9]{2,4})*) Example
All rights reserved\.`

func replacementConfig(t *testing.T, pattern string) *ConfigHeader {
	t.Helper()
	cfg := &ConfigHeader{
		License: LicenseConfig{Content: newNotice, Pattern: validNotice, ReplacePattern: pattern},
		Paths:   []string{"**"},
		Comment: Never,
	}
	require.NoError(t, cfg.Finalize())
	return cfg
}

func renderNotice(t *testing.T, filename, body string) string {
	t.Helper()
	value, err := GenerateLicenseHeader(comments.FileCommentStyle(filename), &ConfigHeader{
		License: LicenseConfig{Content: body},
	})
	require.NoError(t, err)
	return strings.TrimSuffix(value, "\n\n")
}

func TestReplaceKnownHeader(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		preamble string
		year     string
		crlf     bool
	}{
		{name: "block", filename: "source.cpp", year: "2018"},
		{name: "range", filename: "source.cpp", year: "2018-2024"},
		{name: "year list", filename: "source.cpp", year: "2018, 2020"},
		{name: "short range", filename: "source.cpp", year: "2018-24"},
		{name: "line", filename: "source.go", year: "2018"},
		{name: "python", filename: "source.py", preamble: "#!/usr/bin/env python3\n# coding: utf-8\n\n", year: "2018"},
		{name: "powershell", filename: "source.ps1", preamble: "#!/usr/bin/env pwsh\n", year: "2018"},
		{name: "php", filename: "source.php", preamble: "<?php\n", year: "2018"},
		{name: "xml", filename: "source.xml", preamble: "<?xml version=\"1.0\"?>\n", year: "2018"},
		{name: "crlf", filename: "source.cpp", year: "2018-2024", crlf: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			old := renderNotice(t, tt.filename, strings.ReplaceAll(oldNotice, "2018", tt.year))
			suffix := "\n\n// Unrelated ordinary comment.\nint café = 7;\n"
			input := tt.preamble + old + suffix
			if tt.crlf {
				input = strings.ReplaceAll(input, "\n", "\r\n")
				suffix = strings.ReplaceAll(suffix, "\n", "\r\n")
			}
			file := filepath.Join(t.TempDir(), tt.filename)
			require.NoError(t, os.WriteFile(file, []byte(input), 0o600))
			cfg := replacementConfig(t, oldNoticePattern)
			var before Result
			require.NoError(t, CheckFile(file, cfg, &before))
			require.True(t, before.HasFailure())
			var fixed Result
			require.NoError(t, Fix(file, cfg, &fixed))
			require.Equal(t, []string{file}, fixed.Fixed)
			output, err := os.ReadFile(file)
			require.NoError(t, err)
			expected := renderNotice(t, tt.filename, strings.ReplaceAll(newNotice, "[year]", tt.year))
			if tt.crlf {
				expected = strings.ReplaceAll(expected, "\n", "\r\n")
			}
			require.Equal(t, tt.preamble+expected+suffix, string(output))
			require.Equal(t, 1, bytes.Count(output, []byte("Copyright")))
			require.NotContains(t, string(output), "CONFIDENTIAL")
			require.Equal(t, "", cfg.License.CopyrightYear)
			var checked Result
			require.NoError(t, CheckFile(file, cfg, &checked))
			require.False(t, checked.HasFailure())
			var again Result
			require.NoError(t, Fix(file, cfg, &again))
			unchanged, err := os.ReadFile(file)
			require.NoError(t, err)
			require.Equal(t, output, unchanged)
		})
	}
}

func TestReplacePreservesEncoding(t *testing.T) {
	tests := []struct {
		name  string
		bom   []byte
		order binary.ByteOrder
	}{
		{name: "UTF-8 BOM", bom: utf8BOM},
		{name: "UTF-16LE", bom: utf16LEBOM, order: binary.LittleEndian},
		{name: "UTF-16BE", bom: utf16BEBOM, order: binary.BigEndian},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := renderNotice(t, "source.cpp", oldNotice) + "\n\nint café;\n"
			encoded := append(append([]byte{}, tt.bom...), []byte(input)...)
			if tt.order != nil {
				encoded = encodeUTF16(input, tt.order, tt.bom)
			}
			file := filepath.Join(t.TempDir(), "source.cpp")
			require.NoError(t, os.WriteFile(file, encoded, 0o600))
			cfg := replacementConfig(t, oldNoticePattern)
			var result Result
			require.NoError(t, Fix(file, cfg, &result))
			output, err := os.ReadFile(file)
			require.NoError(t, err)
			require.True(t, bytes.HasPrefix(output, tt.bom))
			expected := renderNotice(t, "source.cpp", strings.ReplaceAll(newNotice, "[year]", "2018")) + "\n\nint café;\n"
			if tt.order == nil {
				require.Equal(t, expected, string(output[len(tt.bom):]))
			} else {
				require.Equal(t, expected, decodeUTF16(t, output[len(tt.bom):], tt.order))
			}
		})
	}
}

func TestReplaceMissingOrValidHeader(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantChanged bool
	}{
		{name: "missing", input: "int value;\n", wantChanged: true},
		{name: "package named license", input: "package license\n\nfunc F() {}\n", wantChanged: true},
		{name: "code with parenthesized argument", input: "def f(c):\n    return c\n", wantChanged: true},
		{name: "ordinary comment", input: "/* Explains the algorithm. */\nint value;\n", wantChanged: true},
		{name: "valid historical", input: renderNotice(t, "source.cpp", strings.ReplaceAll(newNotice, "[year]", "2010")) + "\n\nint value;\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "source.cpp")
			require.NoError(t, os.WriteFile(file, []byte(tt.input), 0o600))
			var result Result
			require.NoError(t, Fix(file, replacementConfig(t, oldNoticePattern), &result))
			output, err := os.ReadFile(file)
			require.NoError(t, err)
			if tt.wantChanged {
				require.Contains(t, string(output), time.Now().Format("2006"))
				require.True(t, bytes.HasSuffix(output, []byte(tt.input)))
			} else {
				require.Equal(t, tt.input, string(output))
				require.Empty(t, result.Fixed)
			}
		})
	}
}

func TestReplaceRejectsUnsafeInput(t *testing.T) {
	old := renderNotice(t, "source.cpp", oldNotice)
	tests := []struct {
		name              string
		input             string
		pattern           string
		clearValidPattern bool
		want              string
	}{
		{name: "unknown owner", input: "/* Copyright 2018 Someone Else. */\nint value;\n", pattern: oldNoticePattern, want: "unrecognized"},
		{name: "foreign style", input: "// Copyright 2018 Someone Else.\nint value;\n", pattern: oldNoticePattern, want: "unrecognized"},
		{name: "partial pattern", input: old + "\nint value;\n", pattern: "Copyright.*", want: "unrecognized"},
		{
			name: "extra body indentation", input: strings.ReplaceAll(old, "Copyright", "  Copyright"),
			pattern: oldNoticePattern, want: "unrecognized",
		},
		{name: "duplicate", input: old + "\n\n" + old + "\nint value;\n", pattern: oldNoticePattern, want: "multiple"},
		{
			name: "notice after ordinary comment", input: "/* " + strings.Repeat("ordinary ", 30) + "*/\n" + old + "\nint value;\n",
			pattern: oldNoticePattern, want: "unrecognized",
		},
		{
			name: "mixed style duplicate", input: old + "\n// SPDX-License-Identifier: LicenseRef-Proprietary\nint value;\n",
			pattern: oldNoticePattern, want: "multiple",
		},
		{
			name: "proprietary shorthand", input: "/* (C) 2018 Example Corp. All Rights Reserved. Proprietary. */\nint value;\n",
			pattern: oldNoticePattern, want: "unrecognized",
		},
		{name: "unterminated", input: "/* Copyright 2018 Example\nint value;\n", pattern: oldNoticePattern, want: "unterminated"},
		{
			name: "invalid captured year", input: strings.ReplaceAll(old, "2018", "invalid"),
			pattern: strings.ReplaceAll(oldNoticePattern, "[0-9]{4}(?:[-, ]+[0-9]{2,4})*", "invalid"), want: "invalid year",
		},
		{name: "incompatible preserved year", input: old + "\nint value;\n", pattern: oldNoticePattern, clearValidPattern: true, want: "does not satisfy"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := replacementConfig(t, tt.pattern)
			if tt.clearValidPattern {
				cfg.License.Pattern = ""
			}
			file := filepath.Join(t.TempDir(), "source.cpp")
			require.NoError(t, os.WriteFile(file, []byte(tt.input), 0o600))
			var result Result
			require.ErrorContains(t, Fix(file, cfg, &result), tt.want)
			output, err := os.ReadFile(file)
			require.NoError(t, err)
			require.Equal(t, tt.input, string(output))
			require.Empty(t, result.Fixed)
		})
	}
}

func TestReplacementPatternValidation(t *testing.T) {
	for _, pattern := range []string{"[", ".*", "^$", "CONFIDENTIAL)|(?:.+"} {
		t.Run(pattern, func(t *testing.T) {
			cfg := &ConfigHeader{License: LicenseConfig{ReplacePattern: pattern}}
			require.ErrorContains(t, cfg.Finalize(), "replace-pattern")
		})
	}
}

func TestReplacementWithoutYearCapture(t *testing.T) {
	cfg := replacementConfig(t, strings.ReplaceAll(oldNoticePattern, "(?P<year>", "("))
	cfg.License.CopyrightYear = "2001"
	file := filepath.Join(t.TempDir(), "source.cpp")
	require.NoError(t, os.WriteFile(file, []byte(renderNotice(t, "source.cpp", oldNotice)), 0o600))
	var result Result
	require.NoError(t, Fix(file, cfg, &result))
	output, err := os.ReadFile(file)
	require.NoError(t, err)
	require.Contains(t, string(output), "Copyright 2001 Example")
}

func TestReplacementDoesNotChangeLegacyDefaults(t *testing.T) {
	for _, input := range []string{"int value;\n", renderNotice(t, "source.cpp", oldNotice)} {
		cfg := replacementConfig(t, "")
		file := filepath.Join(t.TempDir(), "source.cpp")
		require.NoError(t, os.WriteFile(file, []byte(input), 0o600))
		generated, err := GenerateLicenseHeader(comments.FileCommentStyle(file), cfg)
		require.NoError(t, err)
		expected := rewriteContent(comments.FileCommentStyle(file), []byte(input), generated, cfg.LicensePattern(comments.FileCommentStyle(file)))
		var result Result
		require.NoError(t, Fix(file, cfg, &result))
		output, err := os.ReadFile(file)
		require.NoError(t, err)
		require.Equal(t, expected, output)
		require.Equal(t, []string{file}, result.Fixed)
	}
}

func TestReplacementYearAlternatives(t *testing.T) {
	cfg := replacementConfig(t, oldNoticePattern+`|Copyright \(c\) (?P<year>[0-9]{4}) Example Corp\.`)
	cfg.License.CopyrightYear = "2001"
	input := renderNotice(t, "source.cpp", "Copyright (c) 2015 Example Corp.")
	file := filepath.Join(t.TempDir(), "source.cpp")
	require.NoError(t, os.WriteFile(file, []byte(input), 0o600))
	var result Result
	require.NoError(t, Fix(file, cfg, &result))
	output, err := os.ReadFile(file)
	require.NoError(t, err)
	require.Contains(t, string(output), "Copyright 2015 Example")
}

func TestReplacementRejectsConflictingYearCaptures(t *testing.T) {
	cfg := replacementConfig(t, `CONFIDENTIAL (?P<year>2018) (?P<year>2024)`)
	input := renderNotice(t, "source.cpp", "CONFIDENTIAL 2018 2024")
	file := filepath.Join(t.TempDir(), "source.cpp")
	require.NoError(t, os.WriteFile(file, []byte(input), 0o600))
	var result Result
	require.ErrorContains(t, Fix(file, cfg, &result), "conflicting year")
	output, err := os.ReadFile(file)
	require.NoError(t, err)
	require.Equal(t, input, string(output))
}

func TestReplacementSingleLineCRLF(t *testing.T) {
	for _, filename := range []string{"source.go", "source.cpp"} {
		t.Run(filename, func(t *testing.T) {
			cfg := replacementConfig(t, `CONFIDENTIAL Copyright (?P<year>[0-9]{4}) Example All rights reserved\.`)
			input := renderNotice(t, filename, "CONFIDENTIAL Copyright 2018 Example All rights reserved.")
			input = strings.ReplaceAll(input, "\n", "\r\n") + "\r\nint value;\r\n"
			file := filepath.Join(t.TempDir(), filename)
			require.NoError(t, os.WriteFile(file, []byte(input), 0o600))
			var result Result
			require.NoError(t, Fix(file, cfg, &result))
			output, err := os.ReadFile(file)
			require.NoError(t, err)
			require.NotContains(t, strings.ReplaceAll(string(output), "\r\n", ""), "\n")
		})
	}
}

func TestReplacementOptionalYearFallback(t *testing.T) {
	for _, year := range []string{"", "2001"} {
		t.Run(year, func(t *testing.T) {
			cfg := replacementConfig(t, oldNoticePattern+"|OLD NOTICE")
			cfg.License.CopyrightYear = year
			input := renderNotice(t, "source.cpp", "OLD NOTICE")
			file := filepath.Join(t.TempDir(), "source.cpp")
			require.NoError(t, os.WriteFile(file, []byte(input), 0o600))
			var result Result
			require.NoError(t, Fix(file, cfg, &result))
			output, err := os.ReadFile(file)
			require.NoError(t, err)
			expected := year
			if expected == "" {
				expected = time.Now().Format("2006")
			}
			require.Contains(t, string(output), "Copyright "+expected+" Example")
		})
	}
}

func TestReplacementMissingPHPHeader(t *testing.T) {
	input := "<p>Hello</p>\n"
	file := filepath.Join(t.TempDir(), "source.php")
	require.NoError(t, os.WriteFile(file, []byte(input), 0o600))
	var result Result
	require.NoError(t, Fix(file, replacementConfig(t, oldNoticePattern), &result))
	output, err := os.ReadFile(file)
	require.NoError(t, err)
	require.True(t, bytes.HasPrefix(output, []byte("<?php\n")))
	require.True(t, bytes.HasSuffix(output, []byte("?>"+input)))
}

func TestReplacementTrimsTrailingHeaderSpace(t *testing.T) {
	cfg := replacementConfig(t, oldNoticePattern)
	input := renderNotice(t, "source.cpp", strings.ReplaceAll(oldNotice, "Example", "Example  \t"))
	file := filepath.Join(t.TempDir(), "source.cpp")
	require.NoError(t, os.WriteFile(file, []byte(input), 0o600))
	var result Result
	require.NoError(t, Fix(file, cfg, &result))
	output, err := os.ReadFile(file)
	require.NoError(t, err)
	require.Contains(t, string(output), "Copyright 2018 Example")
	require.NotContains(t, string(output), "Example  \t")
}

func TestReplacementBeforePackageNamedLicense(t *testing.T) {
	input := renderNotice(t, "source.go", oldNotice) + "\n\npackage license\n"
	file := filepath.Join(t.TempDir(), "source.go")
	require.NoError(t, os.WriteFile(file, []byte(input), 0o600))
	var result Result
	require.NoError(t, Fix(file, replacementConfig(t, oldNoticePattern), &result))
	output, err := os.ReadFile(file)
	require.NoError(t, err)
	require.Contains(t, string(output), "Copyright 2018 Example")
	require.True(t, bytes.HasSuffix(output, []byte("\n\npackage license\n")))
}
