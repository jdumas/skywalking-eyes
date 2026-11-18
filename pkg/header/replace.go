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
	"fmt"
	"regexp"
	"strings"

	"github.com/apache/skywalking-eyes/pkg/comments"
	lcs "github.com/apache/skywalking-eyes/pkg/license"
)

var (
	replacementYear = regexp.MustCompile(`^\d{4}(?:[-, ]+\d{2,4})*$`)
	noticeMarker    = regexp.MustCompile(`(?i)\b(?:copyright|confidential|licensed|licen[cs]e|proprietary)\b|` +
		`all rights reserved|SPDX-License-Identifier|©|\(c\)`)
	rawNoticeMarker = regexp.MustCompile(`(?i)\b(?:copyright|confidential)\b|all rights reserved|SPDX-License-Identifier|©`)
)

func (config *ConfigHeader) replacementPattern() (*regexp.Regexp, error) {
	raw := strings.TrimSpace(config.License.ReplacePattern)
	if _, err := regexp.Compile(raw); err != nil {
		return nil, fmt.Errorf("invalid license.replace-pattern: %w", err)
	}
	pattern, err := regexp.Compile(`(?s)\A(?:` + raw + `)\z`)
	if err != nil {
		return nil, fmt.Errorf("invalid license.replace-pattern: %w", err)
	}
	if pattern.MatchString("") {
		return nil, fmt.Errorf("license.replace-pattern must not match an empty header")
	}
	return pattern, nil
}

func replaceKnownHeader(content []byte, style *comments.CommentStyle, config *ConfigHeader) ([]byte, error) {
	pattern, err := config.replacementPattern()
	if err != nil {
		return nil, err
	}
	start := headerStart(content, style)
	end, body, found, err := leadingComment(content, start, style)
	if err != nil {
		return nil, err
	}
	local := *config
	match := pattern.FindStringSubmatch(body)
	if found && match != nil {
		if noticeErr := nearbyNotice(content, end, style, config.LicenseLocationThreshold); noticeErr != nil {
			return nil, fmt.Errorf("multiple or malformed leading notices: %w", noticeErr)
		}
		year := ""
		for index, name := range pattern.SubexpNames() {
			if name != "year" || match[index] == "" {
				continue
			}
			if !replacementYear.MatchString(match[index]) {
				return nil, fmt.Errorf("invalid year capture in license.replace-pattern")
			}
			if year != "" && year != match[index] {
				return nil, fmt.Errorf("conflicting year captures in license.replace-pattern")
			}
			year = match[index]
		}
		if year != "" {
			local.License.CopyrightYear = year
		}
	} else {
		if noticeErr := nearbyNotice(content, start, style, config.LicenseLocationThreshold); noticeErr != nil {
			return nil, fmt.Errorf("unrecognized leading license notice: %w", noticeErr)
		}
	}
	generated, err := GenerateLicenseHeader(style, &local)
	if err != nil {
		return nil, err
	}
	var output []byte
	if found && match != nil {
		generated = strings.TrimSuffix(generated, "\n\n")
		if bytes.Contains(content[start:end], []byte("\r\n")) || bytes.HasPrefix(content[end:], []byte("\r\n")) {
			generated = strings.ReplaceAll(generated, "\n", "\r\n")
		}
		output = append(output, content[:start]...)
		output = append(output, generated...)
		output = append(output, content[end:]...)
	} else {
		output = rewriteContent(style, content, generated, nil)
	}
	if !satisfy(lcs.NormalizeHeader(string(output)), config, config.NormalizedLicense(), config.NormalizedPattern()) {
		return nil, fmt.Errorf("replacement does not satisfy the configured license; check pattern and year capture")
	}
	return output, nil
}

func nearbyNotice(content []byte, position int, style *comments.CommentStyle, limit int) error {
	for position < len(content) {
		position = skipHeaderSpace(content, position)
		end, body, found, err := leadingComment(content, position, style)
		if err != nil {
			return err
		}
		if !found {
			prefix := content[position:]
			if len(prefix) > limit {
				prefix = prefix[:limit]
			}
			if rawNoticeMarker.Match(prefix) {
				return fmt.Errorf("nearby license-like text")
			}
			return nil
		}
		if noticeMarker.MatchString(body) {
			return fmt.Errorf("license-like comment")
		}
		if end <= position {
			return fmt.Errorf("comment parser made no progress")
		}
		position = end
	}
	return nil
}

func headerStart(content []byte, style *comments.CommentStyle) int {
	offset := 0
	if style.After != "" {
		if location := regexp.MustCompile(style.After).FindIndex(content); len(location) == 2 &&
			len(bytes.TrimSpace(content[:location[0]])) == 0 {
			offset = location[1]
		}
	}
	return skipHeaderSpace(content, offset)
}

func skipHeaderSpace(content []byte, offset int) int {
	return len(content) - len(bytes.TrimLeft(content[offset:], " \t\r\n"))
}

func leadingComment(content []byte, start int, style *comments.CommentStyle) (end int, body string, found bool, err error) {
	opener, middle, closer := strings.TrimSpace(style.Start), strings.TrimSpace(style.Middle), strings.TrimSpace(style.End)
	if !bytes.HasPrefix(content[start:], []byte(opener)) {
		return start, "", false, nil
	}
	var lines []string
	end = start
	if opener == middle && middle == closer {
		for position := start; position < len(content); {
			rest := content[position:]
			length := bytes.IndexByte(rest, '\n')
			if length < 0 {
				length = len(rest)
			}
			line := strings.TrimSuffix(string(rest[:length]), "\r")
			trimmed := strings.TrimLeft(line, " \t")
			if !strings.HasPrefix(trimmed, opener) {
				break
			}
			lines = append(lines, strings.TrimRight(strings.TrimPrefix(strings.TrimPrefix(trimmed, opener), " "), " \t"))
			end = position + len(line)
			position += length
			if position < len(content) {
				position++
			}
		}
	} else {
		if closer == "" {
			return start, "", false, fmt.Errorf("replacement requires a delimited comment style")
		}
		position := bytes.Index(content[start+len(opener):], []byte(closer))
		if position < 0 {
			return start, "", false, fmt.Errorf("unterminated leading comment")
		}
		position += start + len(opener)
		text := string(content[start+len(opener) : position])
		for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
			trimmed := strings.TrimLeft(line, " \t")
			if middle != "" {
				trimmed = strings.TrimPrefix(trimmed, middle)
			}
			lines = append(lines, strings.TrimRight(strings.TrimPrefix(trimmed, " "), " \t"))
		}
		end = position + len(closer)
	}
	return end, strings.TrimSpace(strings.Join(lines, "\n")), true, nil
}
