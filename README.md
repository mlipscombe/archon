# Archon

[![CI](https://github.com/mlipscombe/archon/actions/workflows/ci.yml/badge.svg)](https://github.com/mlipscombe/archon/actions/workflows/ci.yml)
[![Docker Publish](https://github.com/mlipscombe/archon/actions/workflows/docker-publish.yml/badge.svg)](https://github.com/mlipscombe/archon/actions/workflows/docker-publish.yml)
[![Release](https://github.com/mlipscombe/archon/actions/workflows/release.yml/badge.svg)](https://github.com/mlipscombe/archon/actions/workflows/release.yml)
[![GHCR Archon](https://img.shields.io/badge/ghcr-archon-blue?logo=docker)](https://github.com/users/mlipscombe/packages/container/package/archon)
[![GHCR Sandbox](https://img.shields.io/badge/ghcr-archon--opencode--sandbox-blue?logo=docker)](https://github.com/users/mlipscombe/packages/container/package/archon-opencode-sandbox)

Archon is a local-first autonomous software development orchestrator for Jira + GitHub.

This repository currently implements the MVP spine:

- watch one Jira project
- normalize ticket content
- evaluate readiness with `opencode`
- post clarification comments for `NOT_READY` tickets
- require approval in approval mode
- run implementation in Docker-backed isolated git worktrees
- verify changes before commit/push/PR creation
- open GitHub pull requests
- react to `archon-revise` review loops
- expose a local web UI, health endpoints, metrics, and session history

## MVP Scope

The current build is optimized for a solo developer running Archon locally against:

- one Jira project
- one GitHub repository
- one local repository checkout
- Docker sandbox execution

## Requirements

- Go 1.24+
- Docker
- `opencode` available on your machine or in the sandbox image
- Jira Cloud credentials
- GitHub access via browser OAuth or a token with repo access
- a local clone of the target repository with `origin` configured

## Build

```bash
go build -o bin/archon ./cmd/archon
```

Build the Docker images:

```bash
make docker-build
make docker-build-sandbox
```

Build and push to GHCR manually:

```bash
make docker-push
make docker-push-sandbox
```

## Configure

Generate both configs interactively:

```bash
./bin/archon config
```

This launches a colored interactive wizard built with `survey` and writes:

- user config: `~/.archon/archon.yaml`
- repo config: `./archon.yaml`

The wizard supports these auth styles:

- Jira: `api_token`
- GitHub: `token` or `oauth_browser`

GitHub browser auth uses GitHub's device flow for public clients. It requires only a GitHub OAuth app client ID, opens a browser for user authorization, and stores the resulting token outside `archon.yaml`.
Archon ships with a built-in public GitHub client ID, so the normal setup flow does not ask you to provide one.

You can also start from the checked-in samples:

```bash
mkdir -p ~/.archon
cp archon.user.example.yaml ~/.archon/archon.yaml
cp archon.example.yaml ./archon.yaml
```

Validate the current config:

```bash
./bin/archon config validate
```

If `github.auth_method` is `oauth_browser`, complete the login flow after generating the config:

```bash
./bin/archon auth login github
./bin/archon auth status
```

## Run

```bash
./bin/archon start
```

By default the UI is served at:

```text
http://localhost:8080
```

## Docker Images

This repository now includes two Dockerfiles:

- `Dockerfile`
  - builds the Archon daemon image
  - includes the Archon binary and Docker CLI
- `Dockerfile.sandbox`
  - builds the `archon/opencode-sandbox` runtime image
  - installs `opencode-ai`
  - includes common tooling for implementation runs: `git`, `bash`, `python3`, `node`, `npm`, `go`, `make`, and build tools

Suggested local tags:

```bash
docker build -t ghcr.io/mlipscombe/archon:local -f Dockerfile .
docker build -t ghcr.io/mlipscombe/archon-opencode-sandbox:local -f Dockerfile.sandbox .
```

If you build the sandbox image locally, point your user config at it:

```yaml
sandbox:
  image: ghcr.io/mlipscombe/archon-opencode-sandbox:local
```

CI/CD is configured to:

- run Go and Docker validation on pull requests and pushes to `main`
- publish multi-arch Docker images to GHCR on pushes to `main` and version tags
- publish packaged release binaries on version tags using GoReleaser

If you want to run the Archon daemon in Docker, you will typically need to mount:

- your user config at `~/.archon/archon.yaml`
- your repo-local `./archon.yaml`
- your repository checkout
- `/var/run/docker.sock` so the daemon can launch sandbox containers

## Key Commands

```bash
./bin/archon config
./bin/archon auth login github
./bin/archon auth status
./bin/archon auth logout github
./bin/archon config validate
./bin/archon start
./bin/archon version
```

## Config Notes

Archon loads config in layers:

1. user config: `ARCHON_CONFIG` or `~/.archon/archon.yaml`
2. repo config: nearest `./archon.yaml` found by walking upward from the current working directory
3. environment overrides on top of file/default values

The intended split is:

- `~/.archon/archon.yaml`: secrets, auth, machine-level defaults
- `./archon.yaml`: repo-specific settings intended to be safe to commit

Important MVP requirements enforced by validation:

- exactly one Jira project
- required Jira transitions for `In Progress`, `In Review`, and `Done`
- Docker sandbox enabled
- repository path exists locally

Auth requirements depend on the configured auth mode:

- `jira.auth_method=api_token`: requires `jira.email` and `jira.api_token`
- `github.auth_method=token`: requires `github.token`
- `github.auth_method=oauth_browser`: requires a token store path; Archon uses a built-in public GitHub client ID unless you manually override `github.oauth_browser.client_id`

For `github.auth_method=oauth_browser`, the access token is stored in `github.oauth_browser.token_store_path`, not in `archon.yaml`.

Jira remains token-based for the interactive MVP setup flow.

## Web UI

The local web UI currently provides:

- session list
- session detail page
- evaluation reasoning
- clarification history
- approval / rejection controls
- implementation summary, branch, and PR link
- stored `opencode` runs
- live implementation/revision logs via SSE
- audit trail

## Endpoints

- `/` UI home
- `/sessions/{issueKey}` session detail
- `/sessions/{issueKey}/logs` live log SSE stream
- `/health` dependency status JSON
- `/health/ready` readiness check
- `/metrics` Prometheus-style metrics

## Current Workflow

1. Archon polls Jira using the configured JQL filter.
2. Matching tickets are normalized and persisted.
3. `opencode` evaluates readiness.
4. `NOT_READY` tickets get a new Jira clarification comment.
5. `READY` tickets either:
   - move to `AWAITING_APPROVAL` in approval mode, or
   - move directly to `IMPLEMENTING` in sandbox mode.
6. Implementation runs in a fresh git worktree mounted into Docker.
7. Archon runs verification on the host against that worktree.
8. If verification passes, Archon commits, pushes, and opens a PR.
9. If a PR gets the `archon-revise` label, Archon queues a revision cycle.

## Important Behavior

- Archon owns deterministic git operations.
- `opencode` does not create branches, commit, or push.
- verification failures block commit, push, and PR creation.
- watcher skip logic uses the `archon/{ISSUE_KEY}-*` branch convention.
- live logs are in-memory for the current process; historical inspection comes from persisted run records.

## Troubleshooting

If startup fails, check these first:

- `docker info` works locally
- Jira token, email, and base URL are correct
- GitHub token can access the configured repo
- If using GitHub browser auth, run `./bin/archon auth login github`
- `repo.path` points to a real local checkout
- `origin` exists and accepts pushes

Validate config again:

```bash
./bin/archon config validate
```

Check health:

```bash
curl http://localhost:8080/health
curl http://localhost:8080/health/ready
```

Check metrics:

```bash
curl http://localhost:8080/metrics
```

## Current Gaps

The codebase is still MVP-stage. Notable areas that remain light or incomplete:

- settings editing in the UI
- UI auth polish
- richer repo-specific verification heuristics
- full historical live-log persistence beyond stored run output
- broader deployment and multi-project support

## Repo Files

- `PRD.md` product requirements
- `IMPLEMENTATION_PLAN.md` dependency-ordered build plan
- `archon.example.yaml` repo-safe sample config
- `archon.user.example.yaml` user-level sample config
