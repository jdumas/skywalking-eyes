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

package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/apache/skywalking-eyes/pkg/config"
	"github.com/apache/skywalking-eyes/pkg/header"

	"github.com/stretchr/testify/require"
)

func TestCheckHeaderPathSelection(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, name := range []string{"public/a.go", "public/unselected.go", "public/vendor.go", "private/b.go"} {
		require.NoError(t, os.MkdirAll(filepath.Dir(name), 0o755))
		require.NoError(t, os.WriteFile(name, []byte("// Alpha license\npackage example\n"), 0o600))
	}
	previous := respectConfigPaths
	t.Cleanup(func() { respectConfigPaths = previous })
	tests := []struct {
		name      string
		respect   bool
		paths     []string
		success   []string
		ignored   []string
		wantError bool
	}{
		{
			name:    "intersection",
			respect: true,
			paths:   []string{"public/a.go", "private/b.go", "public/vendor.go"},
			success: []string{"public/a.go"},
			ignored: []string{"private/b.go", "public/vendor.go"},
		},
		{
			name:    "legacy override",
			paths:   []string{"private/b.go"},
			success: []string{"private/b.go"},
		},
		{
			name:    "configured discovery without arguments",
			success: []string{"public/a.go", "public/unselected.go"},
			ignored: []string{"public/vendor.go"},
		},
		{
			name:      "empty intersection fails closed",
			respect:   true,
			wantError: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			respectConfigPaths = tt.respect
			cfg := &header.ConfigHeader{
				License:     header.LicenseConfig{Content: "Alpha license"},
				Paths:       []string{"public/**"},
				PathsIgnore: []string{"public/vendor.go"},
			}
			require.NoError(t, cfg.Finalize())
			var result header.Result
			err := checkHeader(cfg, &result, tt.paths)
			if tt.wantError {
				require.ErrorContains(t, err, "--respect-config-paths requires at least one path")
			} else {
				require.NoError(t, err)
			}
			require.ElementsMatch(t, tt.success, result.Success)
			require.ElementsMatch(t, tt.ignored, result.Ignored)
			require.Empty(t, result.Failure)
			expected := []string{"public/**"}
			if !tt.respect && len(tt.paths) > 0 {
				expected = tt.paths
			}
			require.Equal(t, expected, cfg.Paths)
		})
	}
}

func TestFixRespectsConfiguredHeaders(t *testing.T) {
	t.Chdir(t.TempDir())
	names := []string{"public/a.go", "private/b.go", "public/vendor.go", "public/unselected.go"}
	original := []byte("package example\n")
	for _, name := range names {
		require.NoError(t, os.MkdirAll(filepath.Dir(name), 0o755))
		require.NoError(t, os.WriteFile(name, original, 0o600))
	}
	previousConfig, previousRespect := Config, respectConfigPaths
	t.Cleanup(func() { Config, respectConfigPaths = previousConfig, previousRespect })
	respectConfigPaths = true
	Config = &config.V2{Header: []*header.ConfigHeader{
		{
			License:     header.LicenseConfig{Content: "Alpha license"},
			Paths:       []string{"public/**"},
			PathsIgnore: []string{"public/vendor.go"},
			Comment:     header.Never,
		},
		{
			License:     header.LicenseConfig{Content: "Beta license"},
			Paths:       []string{"**"},
			PathsIgnore: []string{"public/**"},
			Comment:     header.Never,
		},
	}}
	for _, cfg := range Config.Headers() {
		require.NoError(t, cfg.Finalize())
	}
	selected := names[:3]
	require.NoError(t, FixCommand.RunE(FixCommand, selected))
	public, err := os.ReadFile(names[0])
	require.NoError(t, err)
	require.Contains(t, string(public), "Alpha license")
	require.NotContains(t, string(public), "Beta license")
	private, err := os.ReadFile(names[1])
	require.NoError(t, err)
	require.Contains(t, string(private), "Beta license")
	require.NotContains(t, string(private), "Alpha license")
	for _, name := range names[2:] {
		content, err := os.ReadFile(name)
		require.NoError(t, err)
		require.Equal(t, original, content)
	}
	require.NoError(t, CheckCommand.RunE(CheckCommand, selected))
	require.NoError(t, DiffCommand.RunE(DiffCommand, selected))
	require.NoError(t, FixCommand.RunE(FixCommand, selected))
	require.Equal(t, []string{"public/**"}, Config.Headers()[0].Paths)
	require.Equal(t, []string{"**"}, Config.Headers()[1].Paths)
}
