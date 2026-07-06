// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The publisher-pypi Authors

package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	plugin "github.com/SemRels/publisher-pypi/internal/plugin"
)

type stubPublisher struct {
	cfg plugin.Config
	err error
}

func (s *stubPublisher) Publish(_ context.Context, cfg plugin.Config, _, _ io.Writer) error {
	s.cfg = cfg
	return s.err
}

func TestRunPassesConfigToPublisher(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"SEMREL_NEXT_VERSION":               "v1.2.3",
		"SEMREL_DRY_RUN":                    "true",
		"SEMREL_PLUGIN_WORKDIR":             "project",
		"SEMREL_PLUGIN_PYTHON_BIN":          "python3",
		"SEMREL_PLUGIN_TWINE_BIN":           "twine-custom",
		"SEMREL_PLUGIN_PYPI_TOKEN":          "secret-token",
		"SEMREL_PLUGIN_PYPI_REPOSITORY_URL": "https://test.pypi.org/legacy/",
	}

	var stdout, stderr bytes.Buffer
	publisher := &stubPublisher{}
	if code := run(&stdout, &stderr, func(key string) string { return env[key] }, publisher); code != 0 {
		t.Fatalf("run() code = %d stderr=%s", code, stderr.String())
	}

	if publisher.cfg.Version != "1.2.3" || !publisher.cfg.DryRun || publisher.cfg.WorkDir != "project" || publisher.cfg.PythonBin != "python3" || publisher.cfg.TwineBin != "twine-custom" || publisher.cfg.RepositoryURL != "https://test.pypi.org/legacy/" {
		t.Fatalf("unexpected config: %#v", publisher.cfg)
	}
	if !strings.Contains(stderr.String(), "plugin_schema_version=1") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunRequiresVersion(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	if code := run(&stdout, &stderr, func(string) string { return "" }, &stubPublisher{}); code != 1 {
		t.Fatalf("run() code = %d", code)
	}
	if !strings.Contains(stderr.String(), "SEMREL_VERSION is required") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestRunPropagatesPublisherError(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	publisher := &stubPublisher{err: errors.New("boom")}
	if code := run(&stdout, &stderr, func(key string) string {
		if key == "SEMREL_VERSION" {
			return "1.0.0"
		}
		return ""
	}, publisher); code != 1 {
		t.Fatalf("run() code = %d", code)
	}
	if !strings.Contains(stderr.String(), "boom") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}
