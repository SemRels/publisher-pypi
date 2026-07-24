// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The publisher-pypi Authors

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	plugin "github.com/SemRels/publisher-pypi/internal/plugin"
)

const pluginSchemaVersion = 1

type publisher interface {
	Publish(context.Context, plugin.Config, io.Writer, io.Writer) error
}

func main() {
	os.Exit(run(os.Stdout, os.Stderr, os.Getenv, plugin.NewPublisher()))
}

func run(stdout, stderr io.Writer, getenv func(string) string, pub publisher) int {
	_, _ = fmt.Fprintf(stderr, "plugin_schema_version=%d\n", pluginSchemaVersion)

	version := getenv("SEMREL_VERSION")
	if version == "" {
		version = getenv("SEMREL_NEXT_VERSION")
	}
	if version == "" {
		_, _ = fmt.Fprintln(stderr, "publisher-pypi: SEMREL_VERSION is required")
		return 1
	}

	cfg := plugin.Config{
		Version:       strings.TrimPrefix(strings.TrimSpace(version), "v"),
		DryRun:        strings.EqualFold(strings.TrimSpace(getenv("SEMREL_DRY_RUN")), "true"),
		WorkDir:       strings.TrimSpace(getenv("SEMREL_PLUGIN_WORKDIR")),
		PythonBin:     strings.TrimSpace(getenv("SEMREL_PLUGIN_PYTHON_BIN")),
		TwineBin:      strings.TrimSpace(getenv("SEMREL_PLUGIN_TWINE_BIN")),
		Token:         getenv("SEMREL_PLUGIN_PYPI_TOKEN"),
		RepositoryURL: strings.TrimSpace(getenv("SEMREL_PLUGIN_PYPI_REPOSITORY_URL")),
	}

	if err := pub.Publish(context.Background(), cfg, stdout, stderr); err != nil {
		_, _ = fmt.Fprintln(stderr, "publisher-pypi:", err)
		return 1
	}

	return 0
}
