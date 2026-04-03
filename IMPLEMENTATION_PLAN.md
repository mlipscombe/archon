# Archon MVP Implementation Plan

Source of truth: `PRD.md` v1.3

This plan is dependency-ordered. It avoids time estimates on purpose and is optimized for AI-assisted implementation.

## Goal

Build a single-process Go application that can watch one Jira project, evaluate ticket readiness with `opencode`, drive clarification and approval through a mandatory local web UI, implement approved work inside a Docker sandbox backed by isolated git worktrees, run verification, open GitHub pull requests, and handle one revision loop pattern via `archon-revise`.

## Build Principles

- Build the end-to-end loop before polish.
- Keep deterministic actions in Archon: git, worktrees, verification, PR creation, Jira transitions.
- Keep AI work in `opencode`: readiness evaluation, clarification content, implementation, revision edits.
- Prefer the smallest correct architecture over future-proof layering.
- Keep MVP scoped to one Jira project, one GitHub repo, one local operator.
- Use server-rendered UI plus SSE for logs and live updates; avoid a separate SPA/frontend build system.

## MVP End-to-End Slice

The first full slice that matters is:

1. Poll Jira for candidate tickets.
2. Normalize ticket content into plain text.
3. Run readiness evaluation through `opencode`.
4. If not ready, post a new clarification comment and wait.
5. If ready in approval mode, show the plan in the UI and allow approve/reject.
6. Create a fresh worktree and branch.
7. Run `opencode` in Docker against that worktree.
8. Run verification commands.
9. If verification passes, commit, push, create PR, and transition Jira.
10. If review asks for changes and PR gets `archon-revise`, repeat in a fresh worktree from the same branch.

Everything else is support work for this slice.

## Initial Repo Layout

```text
cmd/
  archon/
    main.go

internal/
  app/
    app.go
    runtime.go

  config/
    config.go
    env.go
    validate.go

  db/
    db.go
    migrations/
    sessions_repo.go
    projects_repo.go
    rubric_repo.go
    audit_repo.go

  domain/
    session.go
    issue.go
    evaluation.go
    review.go
    transition.go

  jira/
    client.go
    issues.go
    comments.go
    transitions.go
    adf/
      flatten.go

  github/
    client.go
    prs.go
    reviews.go
    branches.go

  opencode/
    runner.go
    prompts.go
    parser.go
    schema.go

  sandbox/
    docker.go
    logs.go

  gitops/
    worktree.go
    branch.go
    status.go
    commit.go
    push.go

  verify/
    detector.go
    runner.go

  repoctx/
    scan.go
    manifests.go

  watcher/
    poller.go
    debounce.go
    queue.go

  evaluator/
    service.go

  clarification/
    service.go
    comments.go

  approval/
    service.go

  implement/
    service.go

  revision/
    service.go

  state/
    machine.go
    transitions.go

  web/
    server.go
    routes.go
    sse.go
    handlers/
    templates/
    static/

  metrics/
    metrics.go

  logx/
    logger.go
```

## Package Responsibilities

- `internal/app`: process wiring, startup, shutdown, dependency graph.
- `internal/config`: load env and optional YAML, validate required MVP config.
- `internal/db`: SQLite access, migrations, persistence repos.
- `internal/domain`: core types shared across subsystems.
- `internal/jira`: Jira API integration and ADF normalization.
- `internal/github`: GitHub branches, PRs, review polling, labels.
- `internal/opencode`: prompt construction, process invocation, JSON parsing.
- `internal/sandbox`: Docker lifecycle and session log streaming.
- `internal/gitops`: all deterministic git operations owned by Archon.
- `internal/verify`: detect and run required verification commands.
- `internal/repoctx`: monorepo scanning and prompt context assembly.
- `internal/watcher`: polling, debounce, queueing, update detection.
- `internal/evaluator`: readiness evaluation orchestration.
- `internal/clarification`: Jira clarification cycle behavior.
- `internal/approval`: approve/reject/edit-scope actions.
- `internal/implement`: implementation cycle orchestration.
- `internal/revision`: `archon-revise` cycle orchestration.
- `internal/state`: state machine guards and transition helpers.
- `internal/web`: mandatory local UI, HTTP API, SSE.
- `internal/metrics`: health and Prometheus endpoints.
- `internal/logx`: structured logging helpers.

## Data Model

Start with a narrow schema that serves the MVP loop.

### Core tables

- `projects`
  - one row in MVP
  - stores project key, watch filter, Jira transition names, repo settings
- `sessions`
  - one row per Jira issue lifecycle
  - stores current state, current version, confidence, scope, PR info, timestamps
- `session_events`
  - append-only audit trail of state transitions and operator actions
- `ticket_snapshots`
  - normalized ticket payload by evaluation cycle
- `evaluation_results`
  - parsed evaluator output by cycle
- `clarification_cycles`
  - one row per clarification comment cycle
- `opencode_runs`
  - evaluation / implementation / revision invocations
- `worktrees`
  - path, branch, base ref, lifecycle status for each run
- `pull_requests`
  - PR metadata and revision counters
- `rubric_criteria`
  - seeded default rubric plus runtime edits

### Core enums

- Session states:
  - `OBSERVED`
  - `EVALUATING`
  - `WAITING_FOR_CLARIFICATION`
  - `AWAITING_APPROVAL`
  - `IMPLEMENTING`
  - `REVISING`
  - `PR_OPEN`
  - `FAILED`
  - `COMPLETED`
  - `ABANDONED`
- Run task types:
  - `evaluation`
  - `implementation`
  - `revision`

## Delivery Phases

## Phase 1: Bootstrap

### Scope

- Initialize Go module and build tooling.
- Add config loading from env and `archon.yaml`.
- Initialize SQLite, migrations, and WAL mode.
- Add structured logging.
- Define domain types and state machine contracts.
- Boot the HTTP server shell and a health endpoint.

### Done when

- `archon start` loads config, opens DB, runs migrations, and starts the web server.
- The process fails fast on missing Docker, missing required config, or invalid Jira transition mappings.

## Phase 2: Ticket Intake

### Scope

- Implement Jira client auth and issue search.
- Add poller with high-water mark and debounce.
- Fetch full issue detail and comments.
- Convert ADF to plain text.
- Persist normalized snapshots and observed sessions.

### Done when

- Matching Jira tickets appear in Archon storage and the UI.
- Repeated unchanged poll cycles do not create duplicate work.

## Phase 3: Readiness Evaluation

### Scope

- Seed the default rubric.
- Build `opencode` evaluation prompt template.
- Run `opencode` in Docker and parse structured JSON output.
- Apply confidence threshold override.
- Route to `READY`, `NOT_READY`, or evaluation failure.

### Done when

- A real Jira ticket can be evaluated and its result is visible in the UI.
- Malformed JSON and process failures are persisted and surfaced cleanly.

## Phase 4: Clarification Loop

### Scope

- Render Jira clarification comment bodies from evaluation output.
- Post one new Jira comment per clarification cycle.
- Track which questions were asked and which were answered.
- Re-evaluate on new human comments or description changes.
- Add escalation reminder behavior.

### Done when

- A `NOT_READY` ticket posts a new clarification cycle comment and moves to `WAITING_FOR_CLARIFICATION`.
- A reply or ticket update causes a new evaluation cycle without duplicating already-answered questions.

## Phase 5: Mandatory Web UI

### Scope

- Build a local UI with:
  - Kanban columns
  - session list and detail panel
  - evaluation details
  - clarification history
  - audit trail
  - approve, reject, retry, abandon actions
- Add SSE for session updates and live sandbox logs.

### Done when

- A solo operator can run the whole MVP from the browser.
- CLI remains useful, but no core MVP action requires headless-only flows.

## Phase 6: Worktree and Sandbox Execution

### Scope

- Create fresh git worktrees per implementation and revision run.
- Create or reuse Archon branch names deterministically.
- Mount the worktree into the Docker sandbox.
- Stream `opencode` logs back to the UI.
- Clean up worktrees safely after success or explicit abandonment.

### Done when

- Each run is isolated from the main checkout.
- Multiple historical runs can be audited without corrupting the primary repo.

## Phase 7: Implementation Orchestration

### Scope

- Build implementation prompt template from ticket, approval scope, rubric context, and repo scan.
- Run `opencode` implementation in sandbox.
- Detect verification commands from repo context.
- Run verification after the agent exits.
- Stage, commit, and push only if verification passes.

### Done when

- An approved ticket can become a pushed branch with a structured implementation summary.
- Any verification failure prevents commit, push, and PR creation.

## Phase 8: PR Creation and Jira Transitions

### Scope

- Detect existing Archon work using branch convention only.
- Create GitHub PRs with body template and labels.
- Persist PR metadata.
- Perform required Jira transitions:
  - implementation start -> In Progress
  - PR opened -> In Review
  - PR merged -> Done

### Done when

- A successful implementation run opens a PR and advances Jira state.
- Invalid Jira transitions fail clearly and stop the relevant action.

## Phase 9: Revision Loop

### Scope

- Poll PR review state and labels.
- Trigger revision only when `archon-revise` is applied.
- Build revision prompt from review comments plus prior implementation context.
- Create a fresh worktree from the current Archon branch head.
- Re-run implementation, verification, commit, and push.
- Cap revision cycles.

### Done when

- Review-requested changes can be handled through at least one successful automated revision cycle.

## Phase 10: Hardening

### Scope

- Add `/metrics`, `/health`, and `/health/ready`.
- Add graceful shutdown and restart recovery.
- Improve retry and backoff around Jira, GitHub, Docker, and `opencode`.
- Harden secret redaction in logs and UI.
- Add config validation and clear startup diagnostics.

### Done when

- The app can be stopped and restarted without losing state-machine integrity.
- External API failures degrade one session, not the whole daemon.

## Vertical Milestones

Use these as practical checkpoints instead of dates.

### Milestone A: Observe and Display

- Jira polling works.
- Sessions appear in the UI.
- Audit trail is persisted.

### Milestone B: Evaluate and Clarify

- `opencode` evaluation works.
- Clarification comments post correctly.
- Re-evaluation loop works.

### Milestone C: Approve and Implement

- UI approval works.
- Worktree + sandbox run works.
- Verification gate works.

### Milestone D: Push and Open PR

- Commit/push is Archon-owned.
- PR creation works.
- Jira transitions work.

### Milestone E: Revise and Harden

- `archon-revise` loop works.
- Health/metrics/recovery are in place.

## Recommended Build Order Inside the Codebase

Implement in this order to minimize rework:

1. `config`, `logx`, `db`, `domain`, `state`
2. `web/server` shell and health routes
3. `jira` and `jira/adf`
4. `watcher`
5. `opencode` runner and parser
6. `evaluator`
7. `clarification`
8. UI session list/detail pages
9. `gitops`, `sandbox`, `repoctx`, `verify`
10. `implement`
11. `github`
12. `approval`
13. `revision`
14. `metrics` and shutdown hardening

## What Not To Overbuild

- Do not add multi-project abstractions beyond what the config shape already implies.
- Do not build a separate JS frontend app.
- Do not make `opencode serve` a dependency for MVP.
- Do not add host-mode execution paths.
- Do not build generalized worker distribution or multi-node coordination.
- Do not attempt advanced repo heuristics before simple manifest detection exists.

## Immediate Next Build Steps

1. Scaffold the Go module and directory structure.
2. Create the SQLite schema and migrations.
3. Implement config loading and validation.
4. Stand up the web server shell.
5. Implement Jira polling and normalized ticket persistence.
