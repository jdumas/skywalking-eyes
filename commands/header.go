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
	"errors"

	"github.com/apache/skywalking-eyes/pkg/header"
	"github.com/apache/skywalking-eyes/pkg/logger"

	"github.com/spf13/cobra"
)

var respectConfigPaths bool

var Header = &cobra.Command{
	Use:     "header",
	Aliases: []string{"h"},
	Short:   "License header related commands; e.g. check, fix, etc.",
	Long: "`header` command walks the specified paths and checks if the specified " +
		"files have the license header in the config file. " +
		"Accepts files, directories, and glob patterns.",
}

func init() {
	Header.PersistentFlags().BoolVar(&respectConfigPaths, "respect-config-paths", false,
		"intersect CLI paths with each configured header path filter (requires paths)")
	Header.AddCommand(CheckCommand)
	Header.AddCommand(FixCommand)
	Header.AddCommand(DiffCommand)
}

func checkHeader(config *header.ConfigHeader, result *header.Result, paths []string) error {
	if respectConfigPaths {
		if len(paths) == 0 {
			return errors.New("--respect-config-paths requires at least one path")
		}
		return header.CheckPaths(config, result, paths)
	}
	if len(paths) > 0 {
		logger.Log.Debugln("Overriding paths with command line args.")
		config.Paths = paths
	}
	return header.Check(config, result)
}
