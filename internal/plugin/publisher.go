// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The publisher-pypi Authors

package plugin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const (
	defaultPythonModule = "build"
	defaultWorkDir      = "."
)

// Command describes a subprocess invocation.
type Command struct {
	Name        string
	Args        []string
	Dir         string
	ExtraEnv    []string
	DisplayName string
}

// Runner executes subprocesses.
type Runner interface {
	Run(context.Context, Command, io.Writer, io.Writer) error
}

// LookPathFunc resolves executables from PATH.
type LookPathFunc func(string) (string, error)

// Publisher builds Python distributions and uploads them to PyPI.
type Publisher struct {
	runner   Runner
	lookPath LookPathFunc
}

// Config contains runtime configuration for the plugin.
type Config struct {
	Version       string
	DryRun        bool
	WorkDir       string
	PythonBin     string
	TwineBin      string
	Token         string
	RepositoryURL string
}

// NewPublisher creates a publisher with the default OS-backed dependencies.
func NewPublisher() *Publisher {
	return &Publisher{
		runner:   ExecRunner{},
		lookPath: exec.LookPath,
	}
}

// NewPublisherWithDeps creates a publisher with injected dependencies for tests.
func NewPublisherWithDeps(runner Runner, lookPath LookPathFunc) *Publisher {
	if runner == nil {
		runner = ExecRunner{}
	}
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	return &Publisher{runner: runner, lookPath: lookPath}
}

// Publish builds and optionally uploads Python distributions.
func (p *Publisher) Publish(ctx context.Context, cfg Config, stdout, stderr io.Writer) error {
	if strings.TrimSpace(cfg.Version) == "" {
		return errors.New("SEMREL_VERSION is required")
	}

	cfg.WorkDir = cleanWorkDir(cfg.WorkDir)

	pythonBin, err := p.resolvePython(cfg.PythonBin)
	if err != nil {
		return err
	}

	buildCmd := BuildCommand(cfg.WorkDir, pythonBin)
	if err := p.runner.Run(ctx, buildCmd, stdout, stderr); err != nil {
		return fmt.Errorf("build distributions: %w", err)
	}

	distFiles, err := DistFiles(cfg.WorkDir)
	if err != nil {
		return err
	}

	if cfg.DryRun {
		_, _ = fmt.Fprintf(stdout, "publisher-pypi: [dry-run] built %d artifact(s) for %s\n", len(distFiles), cfg.Version)
		for _, file := range distFiles {
			_, _ = fmt.Fprintf(stdout, "publisher-pypi: [dry-run] would upload %s\n", file)
		}
		return nil
	}

	// Trusted Publishing/OIDC is a future enhancement. The initial plugin keeps
	// auth CI-agnostic by using a standard PyPI API token with twine.
	if strings.TrimSpace(cfg.Token) == "" {
		return errors.New("SEMREL_PLUGIN_PYPI_TOKEN is required unless SEMREL_DRY_RUN=true")
	}

	twineBin, err := p.resolveTwine(cfg.TwineBin)
	if err != nil {
		return err
	}

	uploadCmd := UploadCommand(cfg.WorkDir, twineBin, distFiles, cfg.Token, cfg.RepositoryURL)
	if err := p.runner.Run(ctx, uploadCmd, stdout, stderr); err != nil {
		return fmt.Errorf("upload distributions: %w", err)
	}

	_, _ = fmt.Fprintf(stdout, "publisher-pypi: uploaded %d artifact(s) for %s\n", len(distFiles), cfg.Version)
	return nil
}

// BuildCommand constructs the python build invocation.
func BuildCommand(workDir, pythonBin string) Command {
	return Command{
		Name:        pythonBin,
		Args:        []string{"-m", defaultPythonModule},
		Dir:         cleanWorkDir(workDir),
		DisplayName: pythonBin + " -m " + defaultPythonModule,
	}
}

// UploadCommand constructs the twine upload invocation.
func UploadCommand(workDir, twineBin string, files []string, token, repositoryURL string) Command {
	args := []string{"upload"}
	if trimmed := strings.TrimSpace(repositoryURL); trimmed != "" {
		args = append(args, "--repository-url", trimmed)
	}
	args = append(args, files...)

	return Command{
		Name: twineBin,
		Args: args,
		Dir:  cleanWorkDir(workDir),
		ExtraEnv: []string{
			"TWINE_USERNAME=__token__",
			"TWINE_PASSWORD=" + token,
		},
		DisplayName: twineDisplayName(twineBin, repositoryURL, files),
	}
}

// DistFiles enumerates built files under dist/.
func DistFiles(workDir string) ([]string, error) {
	pattern := filepath.Join(cleanWorkDir(workDir), "dist", "*")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("glob dist artifacts: %w", err)
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("no distribution files found in %s", filepath.Join(cleanWorkDir(workDir), "dist"))
	}

	files := make([]string, 0, len(matches))
	for _, match := range matches {
		info, err := os.Stat(match)
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", match, err)
		}
		if info.IsDir() {
			continue
		}
		files = append(files, filepath.Clean(match))
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no distribution files found in %s", filepath.Join(cleanWorkDir(workDir), "dist"))
	}
	sort.Strings(files)
	return files, nil
}

func (p *Publisher) resolvePython(override string) (string, error) {
	if bin := strings.TrimSpace(override); bin != "" {
		if _, err := p.lookPath(bin); err != nil {
			return "", fmt.Errorf("python executable %q not found on PATH: %w", bin, err)
		}
		return bin, nil
	}

	for _, candidate := range []string{"python", "python3"} {
		if _, err := p.lookPath(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", errors.New("python or python3 must be installed and available on PATH")
}

func (p *Publisher) resolveTwine(override string) (string, error) {
	bin := strings.TrimSpace(override)
	if bin == "" {
		bin = "twine"
	}
	if _, err := p.lookPath(bin); err != nil {
		return "", fmt.Errorf("twine executable %q not found on PATH: %w", bin, err)
	}
	return bin, nil
}

func cleanWorkDir(workDir string) string {
	if strings.TrimSpace(workDir) == "" {
		return defaultWorkDir
	}
	return filepath.Clean(workDir)
}

func twineDisplayName(twineBin, repositoryURL string, files []string) string {
	args := []string{"upload"}
	if trimmed := strings.TrimSpace(repositoryURL); trimmed != "" {
		args = append(args, "--repository-url", trimmed)
	}
	args = append(args, files...)
	return twineBin + " " + strings.Join(args, " ")
}

// ExecRunner executes commands via os/exec.
type ExecRunner struct{}

// Run implements Runner.
func (ExecRunner) Run(ctx context.Context, command Command, stdout, stderr io.Writer) error {
	cmd := exec.CommandContext(ctx, command.Name, command.Args...)
	cmd.Dir = command.Dir
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if len(command.ExtraEnv) > 0 {
		cmd.Env = append(os.Environ(), command.ExtraEnv...)
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", command.DisplayName, err)
	}
	return nil
}
