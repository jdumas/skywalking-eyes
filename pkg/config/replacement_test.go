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

package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseMultipleHeadersWithReplacement(t *testing.T) {
	cfg, err := ParseV2("config.yaml", []byte(`header:
  - license:
      content: "TARGET"
      pattern: "TARGET"
      replace-pattern: "OLD (?P<year>[0-9]{4})"
    paths: ["public/**"]
  - license:
      content: "OTHER"
    paths: ["private/**"]
`))
	require.NoError(t, err)
	require.Equal(t, "OLD (?P<year>[0-9]{4})", cfg.Headers()[0].License.ReplacePattern)
	require.Empty(t, cfg.Headers()[1].License.ReplacePattern)
	require.Equal(t, []string{"public/**"}, cfg.Headers()[0].Paths)
	require.Equal(t, []string{"private/**"}, cfg.Headers()[1].Paths)
}

func TestParseRejectsInvalidReplacementPattern(t *testing.T) {
	_, err := ParseV2("config.yaml", []byte(`header:
  - license:
      content: "TARGET"
      replace-pattern: "["
`))
	require.ErrorContains(t, err, "replace-pattern")
}
