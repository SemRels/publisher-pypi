<!-- SPDX-License-Identifier: Apache-2.0 -->
<!-- SPDX-FileCopyrightText: 2026 The publisher-pypi Authors -->

# publisher-pypi

[![CI](https://github.com/SemRels/publisher-pypi/actions/workflows/ci.yml/badge.svg)](https://github.com/SemRels/publisher-pypi/actions/workflows/ci.yml)
[![Scorecard](https://api.scorecard.dev/projects/github.com/SemRels/publisher-pypi/badge)](https://scorecard.dev/viewer/?uri=github.com/SemRels/publisher-pypi)
[![License](https://img.shields.io/github/license/SemRels/publisher-pypi)](LICENSE)
[![Latest Release](https://img.shields.io/github/v/release/SemRels/publisher-pypi?label=version&color=blue)](https://github.com/SemRels/publisher-pypi/releases/latest)

PyPI publisher plugin for semrel. It builds Python source and wheel distributions with `python -m build` and uploads them with `twine upload`.

This plugin is distributed as a standalone Go binary that semrel runs as a subprocess. The binary reads `SEMREL_*` and `SEMREL_PLUGIN_*` environment variables, announces `plugin_schema_version=1` on stderr, and exits non-zero to abort the release when publishing fails.

## Installation

### Binary

```bash
go install github.com/SemRels/publisher-pypi/cmd/plugin@latest
```

### Docker

Pre-built multi-platform images are published to the GitHub Container Registry on every release:

```bash
docker pull ghcr.io/semrels/publisher-pypi:latest
```

## What it does

1. Detects `python` or `python3` on `PATH`
2. Runs `python -m build` in the configured project directory
3. Enumerates `dist/*`
4. Uploads the files with `twine upload` using a PyPI API token

Dry-run mode still runs the build step so CI can verify packaging metadata without publishing anything.

> Future enhancement: PyPI Trusted Publishing via OIDC is intentionally out of scope for the first version. In practice that flow is tightly coupled to CI providers such as GitHub Actions and usually uses `pypa/gh-action-pypi-publish` rather than generic `twine` subprocess execution.

## Configuration

```yaml
plugins:
  - name: publisher-pypi
    path: ~/.semrel/plugins/semrel-plugin-publisher-pypi
    env:
      SEMREL_PLUGIN_PYPI_TOKEN: "${PYPI_API_TOKEN}"
      # Optional for TestPyPI
      # SEMREL_PLUGIN_PYPI_REPOSITORY_URL: "https://test.pypi.org/legacy/"
```

## `SEMREL_PLUGIN_*` variables

| Name | Required | Description | Default |
| --- | --- | --- | --- |
| `SEMREL_PLUGIN_PYPI_TOKEN` | Yes, unless dry-run | PyPI API token used as `TWINE_PASSWORD`. | _none_ |
| `SEMREL_PLUGIN_PYPI_REPOSITORY_URL` | Optional | Repository URL passed to `twine upload --repository-url`. Use `https://test.pypi.org/legacy/` for TestPyPI. | PyPI default |
| `SEMREL_PLUGIN_WORKDIR` | Optional | Project directory where `python -m build` and `dist/` are evaluated. | `.` |
| `SEMREL_PLUGIN_PYTHON_BIN` | Optional | Override the detected Python executable. | `python` or `python3` |
| `SEMREL_PLUGIN_TWINE_BIN` | Optional | Override the `twine` executable name. | `twine` |

## `SEMREL_*` release context used

| Variable | Description |
| --- | --- |
| `SEMREL_VERSION` | Resolved release version for the current run. |
| `SEMREL_NEXT_VERSION` | Next version computed by semrel for the release. |
| `SEMREL_DRY_RUN` | When `true`, build runs but upload is skipped and planned artifacts are logged. |

## Example behavior

Standard PyPI release:

```bash
SEMREL_VERSION=v1.2.3 \
SEMREL_PLUGIN_PYPI_TOKEN=pypi-... \
semrel-plugin-publisher-pypi
```

TestPyPI release:

```bash
SEMREL_VERSION=v1.2.3 \
SEMREL_PLUGIN_PYPI_TOKEN=pypi-... \
SEMREL_PLUGIN_PYPI_REPOSITORY_URL=https://test.pypi.org/legacy/ \
semrel-plugin-publisher-pypi
```

Dry run:

```bash
SEMREL_VERSION=v1.2.3 \
SEMREL_DRY_RUN=true \
semrel-plugin-publisher-pypi
```

## Development

```bash
go build ./cmd/plugin
go test ./...
```

## License

Apache-2.0
