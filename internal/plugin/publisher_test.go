// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The publisher-pypi Authors

package plugin

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type recordedCall struct {
	command Command
}

type fakeRunner struct {
	calls []recordedCall
	errAt map[int]error
}

func (f *fakeRunner) Run(_ context.Context, command Command, _, _ io.Writer) error {
	idx := len(f.calls)
	f.calls = append(f.calls, recordedCall{command: command})
	if f.errAt == nil {
		return nil
	}
	return f.errAt[idx]
}

func TestBuildCommand(t *testing.T) {
	t.Parallel()

	command := BuildCommand("project", "python3")
	require.Equal(t, "python3", command.Name)
	require.Equal(t, []string{"-m", "build"}, command.Args)
	require.Equal(t, filepath.Clean("project"), command.Dir)
}

func TestUploadCommandIncludesRepositoryURL(t *testing.T) {
	t.Parallel()

	command := UploadCommand("project", "twine", []string{"project/dist/a.whl", "project/dist/a.tar.gz"}, "secret-token", "https://test.pypi.org/legacy/")
	require.Equal(t, []string{"upload", "--repository-url", "https://test.pypi.org/legacy/", "project/dist/a.whl", "project/dist/a.tar.gz"}, command.Args)
	require.Equal(t, []string{"TWINE_USERNAME=__token__", "TWINE_PASSWORD=secret-token"}, command.ExtraEnv)
}

func TestPublishDryRunBuildsAndSkipsUpload(t *testing.T) {
	t.Parallel()

	workDir := prepareDistDir(t, "dryrun", []string{"pkg-1.0.0.tar.gz", "pkg-1.0.0-py3-none-any.whl"})

	runner := &fakeRunner{}
	publisher := NewPublisherWithDeps(runner, func(name string) (string, error) {
		if name == "python" {
			return "/usr/bin/python", nil
		}
		return "", fmt.Errorf("unexpected lookup %s", name)
	})

	var stdout, stderr bytes.Buffer
	err := publisher.Publish(context.Background(), Config{Version: "1.0.0", DryRun: true, WorkDir: workDir}, &stdout, &stderr)
	require.NoError(t, err)
	require.Len(t, runner.calls, 1)
	require.Equal(t, "python", runner.calls[0].command.Name)
	require.Contains(t, stdout.String(), "[dry-run]")
	require.NotContains(t, stdout.String(), "secret-token")
}

func TestPublishUploadsBuiltArtifacts(t *testing.T) {
	t.Parallel()

	workDir := prepareDistDir(t, "upload", []string{"pkg-1.2.3.tar.gz", "pkg-1.2.3-py3-none-any.whl"})

	runner := &fakeRunner{}
	publisher := NewPublisherWithDeps(runner, func(name string) (string, error) {
		switch name {
		case "python", "twine":
			return "/usr/bin/" + name, nil
		default:
			return "", errors.New("not found")
		}
	})

	var stdout, stderr bytes.Buffer
	err := publisher.Publish(context.Background(), Config{
		Version:       "1.2.3",
		WorkDir:       workDir,
		Token:         "secret-token",
		RepositoryURL: "https://test.pypi.org/legacy/",
	}, &stdout, &stderr)
	require.NoError(t, err)
	require.Len(t, runner.calls, 2)
	require.Equal(t, "twine", runner.calls[1].command.Name)
	require.Equal(t, []string{
		"upload",
		"--repository-url", "https://test.pypi.org/legacy/",
		filepath.Clean(filepath.Join(workDir, "dist", "pkg-1.2.3-py3-none-any.whl")),
		filepath.Clean(filepath.Join(workDir, "dist", "pkg-1.2.3.tar.gz")),
	}, runner.calls[1].command.Args)
	require.Contains(t, strings.Join(runner.calls[1].command.ExtraEnv, "\n"), "TWINE_PASSWORD=secret-token")
}

func TestPublishPropagatesBuildFailure(t *testing.T) {
	t.Parallel()

	runner := &fakeRunner{errAt: map[int]error{0: errors.New("exit status 1")}}
	publisher := NewPublisherWithDeps(runner, func(name string) (string, error) {
		if name == "python" {
			return "/usr/bin/python", nil
		}
		return "", errors.New("not found")
	})

	var stdout, stderr bytes.Buffer
	err := publisher.Publish(context.Background(), Config{Version: "1.0.0", DryRun: true}, &stdout, &stderr)
	require.Error(t, err)
	require.Contains(t, err.Error(), "build distributions")
}

func TestPublishRequiresTokenOutsideDryRun(t *testing.T) {
	t.Parallel()

	workDir := prepareDistDir(t, "token", []string{"pkg-1.0.0.tar.gz"})

	runner := &fakeRunner{}
	publisher := NewPublisherWithDeps(runner, func(name string) (string, error) {
		if name == "python" {
			return "/usr/bin/python", nil
		}
		if name == "twine" {
			return "/usr/bin/twine", nil
		}
		return "", errors.New("not found")
	})

	var stdout, stderr bytes.Buffer
	err := publisher.Publish(context.Background(), Config{Version: "1.0.0", WorkDir: workDir}, &stdout, &stderr)
	require.Error(t, err)
	require.Contains(t, err.Error(), "SEMREL_PLUGIN_PYPI_TOKEN is required")
	require.Len(t, runner.calls, 1)
}

func TestPublishPropagatesUploadFailure(t *testing.T) {
	t.Parallel()

	workDir := prepareDistDir(t, "upload-failure", []string{"pkg-1.0.0.tar.gz"})

	runner := &fakeRunner{errAt: map[int]error{1: errors.New("exit status 1")}}
	publisher := NewPublisherWithDeps(runner, func(name string) (string, error) {
		if name == "python" || name == "twine" {
			return "/usr/bin/" + name, nil
		}
		return "", errors.New("not found")
	})

	var stdout, stderr bytes.Buffer
	err := publisher.Publish(context.Background(), Config{
		Version: "1.0.0",
		WorkDir: workDir,
		Token:   "secret-token",
	}, &stdout, &stderr)
	require.Error(t, err)
	require.Contains(t, err.Error(), "upload distributions")
	require.Len(t, runner.calls, 2)
}

func prepareDistDir(t *testing.T, name string, files []string) string {
	t.Helper()

	root := filepath.Join("testdata", name)
	distDir := filepath.Join(root, "dist")
	require.NoError(t, os.RemoveAll(root))
	require.NoError(t, os.MkdirAll(distDir, 0o755))
	for _, file := range files {
		require.NoError(t, os.WriteFile(filepath.Join(distDir, file), []byte("x"), 0o644))
	}
	t.Cleanup(func() {
		require.NoError(t, os.RemoveAll(root))
	})
	return root
}
