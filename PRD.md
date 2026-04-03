# Archon — Product Requirements Document

**Version:** 1.3  
**Status:** Draft  
**Last Updated:** 2026-04-03  
**Owner:** Engineering Leadership

---

## Table of Contents

1. [Executive Summary](#1-executive-summary)
2. [Problem Statement](#2-problem-statement)
3. [Goals & Non-Goals](#3-goals--non-goals)
4. [User Personas](#4-user-personas)
5. [System Architecture Overview](#5-system-architecture-overview)
6. [Core Concepts & Terminology](#6-core-concepts--terminology)
7. [Functional Requirements](#7-functional-requirements)
   - 7.1 [Jira Watcher](#71-jira-watcher)
   - 7.2 [Ticket Reviewer](#72-ticket-reviewer)
   - 7.3 [Requirements Evaluator](#73-requirements-evaluator)
   - 7.4 [Clarification Loop](#74-clarification-loop)
   - 7.5 [Approval Gate](#75-approval-gate)
   - 7.6 [Implementation Orchestrator](#76-implementation-orchestrator)
   - 7.7 [Execution Sandbox](#77-execution-sandbox)
   - 7.8 [PR Manager](#78-pr-manager)
   - 7.9 [Feedback Loop](#79-feedback-loop)
   - 7.10 [State Machine](#710-state-machine)
   - 7.11 [Web UI — Kanban Board](#711-web-ui--kanban-board)
8. [Monorepo Support](#8-monorepo-support)
9. [Non-Functional Requirements](#9-non-functional-requirements)
10. [Configuration & Zero-Config Startup](#10-configuration--zero-config-startup)
11. [Jira Integration Specification](#11-jira-integration-specification)
12. [opencode Integration Specification](#12-opencode-integration-specification)
13. [Execution Sandbox Specification](#13-execution-sandbox-specification)
14. [Readiness Rubric — Database Storage](#14-readiness-rubric--database-storage)
15. [Security & Compliance](#15-security--compliance)
16. [Observability & Monitoring](#16-observability--monitoring)
17. [Error Handling & Recovery](#17-error-handling--recovery)
18. [CLI Interface](#18-cli-interface)
19. [Implementation Checklist](#19-implementation-checklist)
20. [Resolved Design Questions](#20-resolved-design-questions)
21. [Appendix](#21-appendix)

---

## 1. Executive Summary

Archon is an autonomous software development orchestration agent. It watches a configured Jira project for new or updated work items, critically evaluates each ticket for implementation readiness using `opencode` as its reasoning engine, and makes a routing decision:

- **Not ready:** Post targeted clarifying questions as Jira comments — backed by repo-informed suggested answers — park the ticket in a waiting state, and re-evaluate when new information arrives.
- **Ready (approval mode):** Present the evaluation result and implementation plan to a human in the Archon web UI for approval before proceeding.
- **Ready (sandbox mode):** Spawn an `opencode` session inside an isolated Docker container with no approval gate, implement the ticket, and open a pull request automatically.

Archon is designed to require the absolute minimum configuration to get running — a Jira project key, a GitHub token, and an `opencode` installation. Everything else has a sensible default. It runs equally well as a native process on a developer's desktop or as a Docker container in a team environment.

All AI reasoning — ticket evaluation, question generation, answer suggestion, implementation prompt construction, and code generation — flows through `opencode`. Archon does not maintain a separate LLM configuration or API key.

The primary UI is a web-based Kanban board embedded in the Archon process, giving the team real-time visibility into every ticket's status and a set of manual controls for overrides, approvals, and re-evaluation.

For MVP, Archon is optimized for a solo developer running locally against a single Jira project and single GitHub repository. The web UI is mandatory, Docker sandbox execution is required, and every implementation or revision cycle runs in an isolated git worktree managed by Archon.

---

## 2. Problem Statement

### 2.1 The Implementation Readiness Gap

Engineering teams routinely lose time to two failure modes:

1. **Premature implementation:** A developer picks up a vaguely written ticket, makes assumptions, implements the wrong thing, and discovers the mismatch at code review or QA. Rework is expensive.
2. **Idle queue time:** Tickets sit in a backlog waiting for a human to triage, notice missing information, ask the right questions, and wait for a product owner to respond — all through asynchronous Jira comment threads that take days.

### 2.2 The Cost of Context Switching

Senior engineers are frequently pulled in to clarify tickets they did not write, interrupting deep work. This is a high-cost, low-leverage use of their time.

### 2.3 The Clarification Quality Problem

Even when engineers do ask clarifying questions, they often ask from a product perspective, not an implementation one. Archon asks from the codebase's perspective — it knows what patterns exist, what dependencies are available, and what the existing architecture implies — and it surfaces that knowledge as suggested answers alongside each question. This dramatically reduces the round-trip cost of clarification.

### 2.4 The opencode Opportunity

`opencode` provides a programmable, agentic coding interface that can implement well-defined tasks autonomously. The missing link is a reliable upstream gate: something that ensures `opencode` only receives tickets it can actually succeed at, and constructs the implementation prompt with enough precision to yield a usable result.

Archon is that gate.

---

## 3. Goals & Non-Goals

### 3.1 Goals

- Continuously monitor a configured Jira project for actionable tickets in MVP
- Evaluate ticket readiness using `opencode` as the sole AI engine — no separate LLM configuration
- When questions are needed, suggest high-quality answers informed by reading the repository itself
- Support a human-gated approval mode and a fully autonomous sandboxed mode
- Create GitHub pull requests upon successful implementation
- Re-evaluate tickets after comment activity (human responses or new context)
- Maintain a full audit trail of all decisions, evaluations, and session history
- Expose a Kanban-style web UI with manual controls
- Expose a CLI for scripting and terminal-native workflows
- Run on a developer's laptop or in Docker with minimal configuration — under 5 minutes from zero to running
- Handle monorepos gracefully — treat the entire repo as a single unified codebase, cross-package changes expected and supported
- Store all runtime configuration, including the readiness rubric, in Archon's own database
- Optimize MVP for a solo developer running locally against a single Jira project and single GitHub repo

### 3.2 Non-Goals — MVP

- Archon does not perform code review of `opencode` output (that is the responsibility of human PR reviewers)
- Archon does not merge pull requests
- Archon does not manage sprint planning, velocity, or prioritization
- Archon does not support non-Jira issue trackers (Linear, GitHub Issues — future scope)
- Archon does not support non-`opencode` coding agents
- Archon does not maintain a separate LLM API key or provider configuration
- Archon does not process tickets that already have an associated open or merged PR (skipped on first observation)
- Archon does not support GitLab or Bitbucket (GitHub only in MVP)
- Archon does not support assigning sessions to specific machines in a multi-node setup
- Archon does not offer a true dry-run mode (approval mode serves this purpose)
- Archon does not support watching multiple Jira projects or multiple GitHub repositories in MVP
- Archon does not support headless operation without the web UI in MVP

---

## 4. User Personas

### 4.1 The Engineering Lead
Sets up Archon for the team in under 10 minutes. Chooses between approval mode (human gates every implementation) and sandbox mode (fully autonomous). Monitors the Kanban board to track ticket throughput and agent decisions. Uses manual controls to override decisions, force re-evaluation, or abandon sessions.

### 4.2 The Product Owner / Ticket Author
Writes Jira tickets as they always have. Receives comment threads from Archon when a ticket is underspecified, along with suggested answers that give concrete starting points. Responds to Archon's questions and expects implementation to begin automatically once the ticket is ready. Has no direct interaction with Archon's internals.

### 4.3 The Approver (Approval Mode Only)
Receives a notification when Archon has determined a ticket is ready. Reviews Archon's implementation plan and scope summary in the web UI. Clicks Approve to proceed or Reject to send the ticket back with a reason. This persona may be the Engineering Lead, a tech lead, or the ticket author.

### 4.4 The Developer / Reviewer
Receives pull requests created by Archon. Reviews the code as normal. May request changes; Archon picks those up and feeds them back into the `opencode` revision loop. The developer is the final human gate before merge.

### 4.5 The Solo Developer
Runs Archon locally targeting their own Jira board and GitHub repo. Uses sandbox mode. Gets PRs created automatically for well-defined tickets. Archon is effectively a local AI team member. This is the primary MVP persona.

---

## 5. System Architecture Overview

```
┌──────────────────────────────────────────────────────────────────────┐
│                           Archon Process                             │
│                                                                      │
│  ┌─────────────┐   ┌──────────────────┐   ┌──────────────────────┐  │
│  │ Jira Watcher│──▶│ Ticket Reviewer  │──▶│ Requirements         │  │
│  │  (Poller)   │   │  (ADF parser,    │   │ Evaluator            │  │
│  │             │   │   normalizer)    │   │ (via opencode)       │  │
│  └─────────────┘   └──────────────────┘   └──────────┬───────────┘  │
│                                                       │              │
│                                          ┌────────────┴──────────┐   │
│                                          │                       │   │
│                                       READY                  NOT_READY
│                                          │                       │   │
│                                          ▼                       ▼   │
│                              ┌────────────────────┐  ┌────────────┐  │
│                              │  Approval Gate     │  │Clarification│  │
│                              │  (mode=approval)   │  │Loop        │  │
│                              │  or auto-proceed   │  │(with repo- │  │
│                              │  (mode=sandbox)    │  │ informed   │  │
│                              └────────┬───────────┘  │ suggested  │  │
│                                       │              │ answers)   │  │
│                              human approves          └─────┬──────┘  │
│                                       │                    │         │
│                                       ▼             human responds   │
│                          ┌────────────────────────┐        │         │
│                          │  Implementation        │        ▼         │
│                          │  Orchestrator          │  ┌───────────┐   │
│                          │  (prompt constructor)  │  │Re-evaluate│   │
│                          └────────────┬───────────┘  └───────────┘   │
│                                       │                              │
│                                       ▼                              │
│                          ┌────────────────────────┐                  │
│                          │  Execution Sandbox     │                  │
│                          │  (Docker container     │                  │
│                          │   running opencode)    │                  │
│                          └────────────┬───────────┘                  │
│                                       │                              │
│                                       ▼                              │
│                          ┌────────────────────────┐                  │
│                          │  PR Manager            │                  │
│                          │  (GitHub API)          │                  │
│                          └────────────────────────┘                  │
│                                                                      │
│  ┌──────────────────────────────────────────────────────────────┐    │
│  │            State Store  (SQLite — zero external dependency)   │    │
│  └──────────────────────────────────────────────────────────────┘    │
│                                                                      │
│  ┌──────────────────────────────────────────────────────────────┐    │
│  │            Web UI  (Kanban Board — served at :8080)           │    │
│  └──────────────────────────────────────────────────────────────┘    │
│                                                                      │
│  ┌──────────────────────────────────────────────────────────────┐    │
│  │            CLI  (archon <command>)                            │    │
│  └──────────────────────────────────────────────────────────────┘    │
└──────────────────────────────────────────────────────────────────────┘
         │                    │                        │
         ▼                    ▼                        ▼
   ┌───────────┐       ┌────────────┐         ┌───────────────┐
   │   Jira    │       │  opencode  │         │    GitHub     │
   │  (REST)   │       │  (process  │         │  (REST API)   │
   │           │       │   in       │         │               │
   └───────────┘       │  Docker)   │         └───────────────┘
                       └────────────┘
```

### 5.1 Key Architectural Decisions

**opencode as the sole AI engine.** Archon does not call any LLM API directly. All AI tasks — readiness evaluation, clarification question generation with suggested answers, and implementation — are delegated to `opencode`. Users configure `opencode` (which they are already using); they do not configure a separate LLM provider in Archon.

**SQLite as the default and only state store.** Zero external dependencies. The database lives at `~/.archon/archon.db` by default. No Postgres, no Redis.

**Web UI served from the same process.** No separate frontend server. The Kanban board is served directly by the Archon daemon on port 8080.

**Sandboxed execution via Docker.** `opencode` runs inside a Docker container with an isolated session worktree mounted as a volume. For MVP, Docker is required and there is no host execution fallback.

**Archon owns deterministic git operations.** Archon, not `opencode`, creates worktrees and branches, runs post-implementation verification commands, creates commits, and pushes branches. `opencode` is responsible for editing code inside the provided worktree and returning structured output.

---

## 6. Core Concepts & Terminology

| Term | Definition |
|---|---|
| **Ticket** | A Jira issue that Archon has been configured to watch |
| **Watch Filter** | A JQL query defining the set of tickets Archon monitors |
| **Readiness Evaluation** | The `opencode`-powered assessment of whether a ticket has sufficient information to implement |
| **Readiness Rubric** | A set of criteria stored in Archon's database that defines "ready" for a project |
| **Clarification Loop** | The state in which Archon has posted questions (with suggested answers) and is waiting for human response |
| **Suggested Answer** | A repo-informed candidate answer to a clarification question, surfaced by Archon to reduce the author's effort |
| **Approval Gate** | In approval mode, the human review step in the web UI before implementation proceeds |
| **Implementation Prompt** | The structured prompt Archon constructs and passes to `opencode` for implementation |
| **Execution Sandbox** | A Docker container in which `opencode` runs to implement a ticket |
| **PR** | Pull request opened by Archon on GitHub |
| **Archon Session** | The full lifecycle of a single ticket from first observation to PR completion or abandonment |
| **State Store** | Archon's SQLite database |
| **Sandbox Mode** | Fully autonomous operating mode; no human approval gate; requires Docker |
| **Approval Mode** | Operating mode in which a human must approve the implementation plan before `opencode` is invoked |

---

## 7. Functional Requirements

### 7.1 Jira Watcher

#### 7.1.1 Overview
The Jira Watcher detects new and updated tickets that match the configured watch filter and enqueues them for evaluation.

#### 7.1.2 Polling
- Archon SHALL poll Jira at a configurable interval (default: 60 seconds) using the Jira REST API v3 with API token auth
- Archon SHALL track `updated` timestamps to avoid reprocessing unchanged tickets
- Archon SHALL detect the following event types:
  - New ticket matching the filter (no prior record in state store)
  - Status transition into a watched status
  - New human comment (non-Archon author)
  - Description or acceptance criteria field update
- Archon SHALL apply a debounce window (default: 30 seconds) to prevent rapid successive updates from triggering parallel evaluations of the same ticket

#### 7.1.3 Ticket Exclusion Rules (MVP Scope Gates)
The following tickets SHALL be silently skipped:

- Tickets that already have an open or merged pull request in the target GitHub repository. Detection: scan GitHub for branches matching `archon/{issue-key}-*`. In MVP, branch naming convention is the sole ticket-to-PR linkage mechanism.
- Tickets labeled `archon-skip`
- Tickets already in an active Archon session (unless a re-evaluation trigger is detected)

#### 7.1.4 Default Watch Filter
If no JQL is explicitly configured:
```
project = {PROJECT_KEY}
AND issuetype in (Story, Task, Bug)
AND status = "Ready for Dev"
AND assignee is EMPTY
```

---

### 7.2 Ticket Reviewer

#### 7.2.1 Overview
Fetches full ticket detail from Jira and normalizes it into a structured plain-text representation for `opencode` consumption.

#### 7.2.2 Data Fetched and Normalized

| Field | Jira Source | Notes |
|---|---|---|
| Issue key | `key` | |
| Summary | `fields.summary` | |
| Description | `fields.description` | ADF → plain text |
| Issue type | `fields.issuetype.name` | |
| Priority | `fields.priority.name` | |
| Labels | `fields.labels` | |
| Components | `fields.components` | |
| Acceptance criteria | Configurable custom field | |
| Story points | Configurable custom field | |
| Epic link / parent | `fields.parent` | Key + summary |
| Comments | `fields.comment.comments[]` | Full thread, oldest first |
| Linked issues | `fields.issuelinks[]` | Key + summary only |

#### 7.2.3 ADF Parsing
- Convert Atlassian Document Format to clean plain text
- Preserve code blocks, ordered/unordered lists, and table structure
- Strip inline formatting marks (bold, italic) from prose to reduce token noise

---

### 7.3 Requirements Evaluator

#### 7.3.1 Overview
The Requirements Evaluator is the core intelligence of Archon. It invokes `opencode` with the normalized ticket, the stored readiness rubric, and access to the repository, and receives a structured evaluation result.

#### 7.3.2 Evaluation via opencode
- Archon constructs an evaluation prompt and passes it to `opencode`
- `opencode` has access to the full repository through the isolated sandbox-mounted worktree and is expected to read relevant code when formulating questions and suggested answers
- `opencode` returns a structured JSON evaluation result
- Archon parses the result and routes the ticket

#### 7.3.3 Evaluation Output Schema
```json
{
  "decision": "READY" | "NOT_READY",
  "confidence": 0.85,
  "reasoning": "Plain text explanation of the decision",
  "missing_elements": ["Specific missing items when NOT_READY..."],
  "clarifying_questions": [
    {
      "question": "Which user roles should have access to this endpoint?",
      "suggested_answer": "Based on the existing RBAC implementation in src/auth/roles.ts, the pattern used on similar endpoints is hasRole('admin'). If this endpoint should follow the same pattern, the answer is: admin only. Confirm or specify additional roles.",
      "rationale": "The endpoint touches user data — access control is required and not specified"
    }
  ],
  "implementation_notes": ["Key considerations when READY..."],
  "suggested_scope": "Plain text description of what Archon understands to be in scope",
  "out_of_scope_assumptions": ["Items explicitly treated as out of scope"]
}
```

#### 7.3.4 Repo-Informed Suggested Answers
This is a distinguishing capability of Archon. When generating clarifying questions, `opencode` SHALL search the repository for existing patterns relevant to each question and formulate a `suggested_answer` as repo-aware prose grounded in concrete files, functions, modules, or conventions already present in the codebase.

The suggested answer is framed as a starting point: *"Based on X in the codebase, the likely answer is Y — please confirm or correct."*

This reduces clarification round-trips dramatically: instead of a multi-day back-and-forth, the ticket author often needs only to confirm or lightly adjust a pre-filled answer.

#### 7.3.5 Confidence Threshold
- If `confidence < 0.7`, treat as `NOT_READY` regardless of the `decision` field
- Threshold is configurable

#### 7.3.6 Prior Context Handling
- Prior evaluation results, questions, and all subsequent human comment responses are included in the evaluation context on re-evaluation
- Archon SHALL NOT re-ask questions that have already been answered
- Archon detects when all prior questions have been addressed and returns `READY` if the rubric is now satisfied

---

### 7.4 Clarification Loop

#### 7.4.1 Overview
When the Evaluator returns `NOT_READY`, Archon posts a structured comment to the Jira ticket containing its questions and repo-informed suggested answers, then parks the ticket in a waiting state.

#### 7.4.2 Comment Format
```
👋 *Archon needs a few things clarified before implementation can begin.*

---

**Question 1:** Which user roles should have access to this endpoint?

> 💡 *Suggested answer (from codebase):* Based on the existing RBAC pattern in `src/auth/roles.ts`, admin-only access is enforced using `hasRole('admin')` on similar endpoints (e.g., `src/api/reports.ts:L42`). If this endpoint should follow the same pattern, the answer is: **admin only**. Confirm or specify if other roles should be included.

---

**Question 2:** Should this feature be gated behind a feature flag?

> 💡 *Suggested answer (from codebase):* The feature flag system in `src/config/flags.ts` uses `isFeatureEnabled(flagKey)`. Several recent features use this pattern (e.g., `GUIDE_ENROLLMENT_V2`). If yes, what should the flag key be?

---

*What Archon understands so far:*
> {suggested_scope}

*Assumed out of scope:*
- {item_1}
- {item_2}

---
_Reply to this comment or update the ticket description. Archon will re-evaluate automatically._
```

#### 7.4.3 Question Quality Requirements
- Questions SHALL be specific and actionable — never generic ("Can you add more detail?")
- Each question SHALL include a `suggested_answer` derived from the repository as repo-aware prose; file or line references are optional when they materially improve clarity
- Maximum 5 questions per comment (most blocking first; a note is added if more exist)
- Archon SHALL NEVER ask a question answerable from existing ticket content or unambiguously from the codebase
- Archon SHALL NEVER re-ask an already-answered question

#### 7.4.4 Waiting State and Re-Evaluation
- After posting, the ticket enters `WAITING_FOR_CLARIFICATION`
- Each clarification cycle SHALL create a new Jira comment rather than editing a previous Archon comment
- Archon monitors for new human comments or description updates
- On qualifying update: re-queue for evaluation with a minimum 60-second delay
- After `escalation_timeout_business_days` (default: 5) with no response: post one follow-up reminder; one per timeout period maximum

---

### 7.5 Approval Gate

#### 7.5.1 Overview
When `mode = approval`, every ticket that reaches `READY` requires explicit human approval via the web UI before `opencode` is invoked.

#### 7.5.2 Approval Notification
When a ticket reaches `READY` in approval mode, Archon SHALL:
1. Move the card to the **Awaiting Approval** Kanban column
2. Post a Jira comment summarizing the intended scope and linking to the Archon UI:

```
✅ *Archon has evaluated this ticket and is ready to implement.*

**Planned scope:**
{suggested_scope}

**Out of scope (by design):**
- {item}

**Implementation notes:**
- {note}

⚠️ *Running in approval mode. Approve or reject this plan in the [Archon dashboard]({ui_url}/sessions/{session_id}).*
```

#### 7.5.3 Approval Actions in the Web UI
From the **Awaiting Approval** card, an authorized user can:
- **Approve** — proceed to implementation immediately
- **Reject with reason** — return to `WAITING_FOR_CLARIFICATION`; rejection reason posted as Jira comment
- **Edit scope then approve** — modify `suggested_scope` before approving; the edited scope is included in the implementation prompt

#### 7.5.4 Sandbox Mode (No Approval Gate)
When `mode = sandbox`, the approval step is bypassed. A ticket that reaches `READY` immediately proceeds to the Execution Sandbox.

Sandbox mode requires Docker. Archon SHALL verify Docker availability at startup when `mode = sandbox` is configured and refuse to start if Docker is not found, with a clear actionable error message.

---

### 7.6 Implementation Orchestrator

#### 7.6.1 Overview
Constructs the implementation prompt, prepares an isolated git worktree, hands execution to the sandbox, and wraps the deterministic git and verification steps around the `opencode` session.

#### 7.6.2 Implementation Prompt Construction

**Directive block:**
```
You are implementing a software change described by the following Jira ticket.
Implement exactly what is described — no more, no less.
Do not introduce unrequested features, speculative refactors, or style changes beyond what is directly necessary.
This is a monorepo. Treat all services, packages, and modules as part of a single unified codebase.
Cross-package changes are permitted and expected when the ticket requires them.
```

**Ticket context block:**
- Issue key, summary, type, priority
- Full normalized description
- Acceptance criteria
- Linked issues (key + summary)

**Archon analysis block:**
- `suggested_scope` (from evaluation, potentially edited by approver)
- `out_of_scope_assumptions`
- `implementation_notes`

**Repository context block:**
- Repository root path and detected structure (top-level dirs and inferred purpose)
- Detected package manager and build system
- Base branch and branch naming convention
- PR template path if `/.github/pull_request_template.md` exists
- Test expectations: "Write or update unit tests for all new public functions and endpoints"
- Commit message convention if detectable (commitlint, conventional commits config)

**Implementation steps:**
```
1. Archon creates an isolated git worktree for the session from {base_branch}
2. Archon creates branch archon/{issue-key}-{slug} in that worktree
3. Implement all changes required by the ticket and scope above inside the provided worktree
4. Write or update tests to cover the new behavior
5. Output a JSON summary of what you changed (schema provided below)
6. Archon runs the required test and verification commands; any failure blocks commit, push, and PR creation
7. Archon creates the commit and pushes the branch; do not commit directly to {base_branch}
```

#### 7.6.3 Branch Naming
Pattern: `archon/{issue-key}-{slug}` where slug is the lowercased, hyphenated ticket summary truncated at 50 characters.  
Example: `archon/ENG-1042-add-role-filter-to-reports-api`

#### 7.6.4 Git Responsibility and Verification
- Archon SHALL create a fresh git worktree for every implementation and revision cycle
- For a first implementation cycle, the worktree is created from the configured base branch
- For a revision cycle, the worktree is created from the current Archon branch head
- Archon SHALL stage changes, create the commit, and push the branch after successful verification
- Archon SHALL block pull request creation if any required test or verification command fails
- A failed verification run marks the session `FAILED`; the branch may remain available for inspection, but no PR is opened

---

### 7.7 Execution Sandbox

#### 7.7.1 Overview
The Execution Sandbox runs `opencode` in a Docker container with an isolated session worktree mounted. It is the required execution environment in MVP for both sandbox mode and approval mode.

#### 7.7.2 Container Configuration
- Base image: `archon/opencode-sandbox:latest` (configurable)
- Volume mount: isolated session worktree at `/workspace` (read-write)
- Network: `bridge` by default (internet access for dependency fetching); `none` available
- Environment injected: `GITHUB_TOKEN`, `OPENCODE_*` settings
- Container destroyed after session completes

#### 7.7.3 Resource Limits
- CPU: 4 cores (configurable)
- Memory: 8GB (configurable)
- Execution timeout: 30 minutes (configurable)

#### 7.7.4 Non-Docker Fallback
- MVP has no host-execution fallback
- If Docker is unavailable, Archon SHALL refuse to start with a clear actionable error message

---

### 7.8 PR Manager

#### 7.8.1 Overview
After `opencode` completes successfully, the PR Manager opens a pull request on GitHub.

#### 7.8.2 PR Fields

| Field | Value |
|---|---|
| Title | `[{issue-key}] {ticket summary}` |
| Base | Configured base branch (default: `main`) |
| Head | `archon/{issue-key}-{slug}` |
| Labels | `archon-generated` (configurable) |
| Draft | Configurable (default: false) |
| Assignees | Ticket assignee if GitHub username mapping is configured |
| Reviewers | Configurable default reviewers |

#### 7.8.3 PR Body Template
```markdown
## Summary

{opencode_implementation_summary}

## Jira Ticket

[{issue-key}]({jira_url}/browse/{issue-key}) — {ticket_summary}

## Acceptance Criteria

- [ ] {criterion_1}
- [ ] {criterion_2}

## Archon Notes

**Scope as implemented:** {suggested_scope}

**Out of scope (by design):** {out_of_scope_assumptions}

**Implementation notes:** {implementation_notes}

---
*This PR was created automatically by Archon. Review carefully before merging.*
```

#### 7.8.4 PR Feedback Loop
- Archon polls open Archon-created PRs for new review comments (default: every 5 minutes)
- On review requesting changes: post Jira comment summarizing feedback
- On `archon-revise` label: extract review comments, construct revision prompt, create a fresh worktree from the same branch, and spawn a new `opencode` session for that revision cycle
- Revision cycles capped at configurable maximum (default: 3) before Archon steps back and posts a human-intervention request

---

### 7.9 Feedback Loop

#### 7.9.1 Event Routing

| Event | Archon Response |
|---|---|
| Human answers clarification comment | Re-evaluate ticket |
| Ticket description updated | Re-evaluate ticket |
| Approval granted | Proceed to implementation |
| Approval rejected | Return to WAITING_FOR_CLARIFICATION; post Jira comment with reason |
| PR review "changes requested" + `archon-revise` label | Trigger revision loop |
| PR merged | Mark COMPLETED; optionally transition Jira to Done |
| PR closed without merge | Post Jira comment; end session |
| Ticket moved to Won't Do / Cancelled | Abandon session; close PR if open |

#### 7.9.2 Required Jira Status Transitions
Required MVP transitions:
- `opencode` spawned → In Progress
- PR opened → In Review
- PR merged → Done

Archon SHALL NOT perform a transition if it is not valid from the ticket's current status.
Archon SHALL refuse the relevant action if a required transition mapping is missing from configuration.

---

### 7.10 State Machine

```
                    ┌────────────┐
                    │  OBSERVED  │
                    └─────┬──────┘
                          │
                          ▼
                    ┌────────────┐
                    │ EVALUATING │◀──────────────────────┐
                    └─────┬──────┘                       │
                          │                              │
             ┌────────────┴─────────────┐                │
             │                          │                │
          READY                     NOT_READY             │
             │                          │                │
             ▼                          ▼                │
  ┌─────────────────────┐   ┌───────────────────────┐    │
  │  AWAITING_APPROVAL  │   │ WAITING_FOR_          │    │
  │  (approval mode)    │   │ CLARIFICATION         │    │
  │  or auto-proceed    │   └───────────┬───────────┘    │
  │  (sandbox mode)     │               │                │
  └──────────┬──────────┘         human responds        │
             │ approved                 └────────────────┘
             │
             ▼
  ┌───────────────────────┐
  │   IMPLEMENTING        │
  │   (opencode in        │
  │    sandbox)           │
  └──────────┬────────────┘
             │
     ┌───────┴────────┐
     │                │
  SUCCESS           FAILURE
     │                │
     ▼                ▼
┌──────────┐    ┌──────────┐
│ PR_OPEN  │    │  FAILED  │
└─────┬────┘    └──────────┘
      │
      ├── archon-revise label
      │           │
      │           ▼
      │     ┌──────────┐
      │     │ REVISING │
      │     └──────────┘
      │
      ├── PR merged ──────────▶ ┌───────────┐
      │                         │ COMPLETED │
      │                         └───────────┘
      │
      └── PR closed / cancelled ▶ ┌──────────┐
                                  │ ABANDONED│
                                  └──────────┘
```

---

### 7.11 Web UI — Kanban Board

#### 7.11.1 Overview
A browser-based Kanban board served by the Archon process at `http://localhost:8080`. This is the primary operational interface and a mandatory part of the MVP experience. The CLI complements the UI but does not replace it.

#### 7.11.2 Kanban Columns

| Column | States Shown |
|---|---|
| **Watching** | OBSERVED, EVALUATING |
| **Needs Clarification** | WAITING_FOR_CLARIFICATION |
| **Awaiting Approval** | AWAITING_APPROVAL |
| **Implementing** | IMPLEMENTING, REVISING |
| **In Review** | PR_OPEN |
| **Done** | COMPLETED (last 30 days) |
| **Failed / Abandoned** | FAILED, ABANDONED |

#### 7.11.3 Ticket Card
Each card displays:
- Jira issue key + summary (linked to Jira)
- Issue type icon and priority indicator
- Time in current state
- Confidence score from last evaluation
- Current state label with color coding
- Context-sensitive quick action buttons

#### 7.11.4 Card Detail Panel
Clicking a card opens a side panel showing:
- Full evaluation result (decision, reasoning, confidence)
- Suggested scope and out-of-scope assumptions
- Clarification questions with suggested answers (if applicable)
- Implementation notes
- Full audit trail (all state transitions with timestamps and actors)
- GitHub PR link (if open)
- Jira ticket link
- Live `opencode` session log (streaming via WebSocket/SSE while in progress)

#### 7.11.5 Controls and Actions

**Global toolbar:**
- Pause / Resume watching
- Mode badge: Approval / Sandbox (read-only; set via config)
- Filter by project, state, or issue type
- Search by issue key or summary

**Per-card actions — approval mode:**

| State | Actions |
|---|---|
| AWAITING_APPROVAL | Approve, Reject (with reason), Edit Scope + Approve |
| WAITING_FOR_CLARIFICATION | Force Re-evaluate, Abandon |
| IMPLEMENTING | View Live Log, Abort |
| PR_OPEN | Trigger Revision, Abandon |
| FAILED | Retry, Abandon |
| Any | View Full Audit Trail |

**Per-card actions — sandbox mode:**

| State | Actions |
|---|---|
| IMPLEMENTING | View Live Log, Abort |
| PR_OPEN | Trigger Revision, Abandon |
| FAILED | Retry, Abandon |
| Any | View Full Audit Trail |

#### 7.11.6 Live Log Streaming
While a session is `IMPLEMENTING` or `REVISING`, the card detail panel streams `opencode` output in real-time via WebSocket or Server-Sent Events.

#### 7.11.7 Settings Panel
The settings panel (accessible from the toolbar) allows:
- Viewing and editing the readiness rubric (per project; changes stored in DB; take effect immediately)
- Viewing configured JQL watch filters
- Viewing Archon version and connection status for Jira and GitHub

Settings that require a restart (port, database path, mode) are not editable in the UI — they require config file changes.

#### 7.11.8 Authentication
- Optional basic auth (username/password in config); off by default (appropriate for local desktop use)
- JWT-based auth is a v1.1 target

---

## 8. Monorepo Support

### 8.1 Philosophy
Archon treats a monorepo as a single unified codebase. There is no concept of service assignment or routing tickets to sub-repo agents. `opencode` receives access to the entire repository and is expected to make cross-package changes as needed — updating a shared library, its consumers, and associated tests in a single session.

### 8.2 Repository Context in Prompts
Archon includes in both evaluation and implementation prompts:
- The full repository root (never a subdirectory)
- Detected package manager and build system (presence of `pnpm-workspace.yaml`, `go.work`, `Cargo.toml`, `pyproject.toml` at root)
- A map of top-level directories with inferred purpose (e.g., `apps/` → applications, `packages/` → shared libraries, `services/` → backend services)
- Workspace member list (from workspace manifest)

This context allows `opencode` to navigate the codebase efficiently without exhaustive exploration.

### 8.3 No Service Routing Required
Tickets do not need to specify which service(s) they affect. `opencode` determines the relevant files and packages by reading the ticket description and searching the codebase. This is a deliberate design choice: forcing ticket authors to pre-identify affected services adds friction and introduces a common failure mode where the author identifies the wrong service.

### 8.4 Cross-Package Testing
The implementation prompt instructs `opencode` to:
- Run the full test suite or the subset relevant to changed packages (detected from per-package test scripts)
- Update shared types, interfaces, or contracts if modified
- Ensure no cross-package regressions

### 8.5 Single Branch, Single PR
One branch and one PR covers all changes across all packages for a given ticket. No multi-PR splitting in MVP.

---

## 9. Non-Functional Requirements

### 9.1 Performance
- Polling cycle (fetch + queue update) completes in under 10 seconds for batches up to 50 tickets
- Concurrent evaluations limited to `max_concurrent_evaluations` (default: 2)
- Archon SHALL NOT block one ticket's evaluation from proceeding because another is in progress

### 9.2 Reliability
- All state transitions persisted before acting on them (write-ahead)
- Exponential backoff with jitter for all external API calls
- A single malformed or failing ticket SHALL NOT crash the daemon or block others
- Graceful shutdown: in-progress sessions allowed to complete or time out (default grace: 5 minutes)

### 9.3 Ease of Operation
- Archon starts with 7 required values (see Section 10); all others have defaults
- First-run web UI wizard guides through minimum required configuration if values are missing
- Docker auto-detected; sandbox activation logged clearly on startup

### 9.4 Portability
- Distributable as a single self-contained binary (Go) or Docker image
- Docker image includes `opencode` pre-installed
- No external runtime dependencies beyond Docker (optional) and configured credentials

---

## 10. Configuration & Zero-Config Startup

### 10.1 Design Principle
A developer should be running Archon within 5 minutes of first encounter. The config file is optional; all required values may be supplied as environment variables. Archon uses convention over configuration throughout.

### 10.2 Required Configuration

MVP supports exactly one Jira project and one GitHub repository.

| Value | Env Var | Config Key | Example |
|---|---|---|---|
| Jira base URL | `JIRA_BASE_URL` | `jira.base_url` | `https://your-org.atlassian.net` |
| Jira email | `JIRA_EMAIL` | `jira.email` | `archon@company.com` |
| Jira API token | `JIRA_API_TOKEN` | `jira.api_token` | (token value) |
| Jira project key | `JIRA_PROJECT_KEY` | `jira.projects[0].key` | `ENG` |
| GitHub token | `GITHUB_TOKEN` | `github.token` | `ghp_...` |
| GitHub repo | `GITHUB_REPO` | `github.repo` | `owner/repo` |
| Repository path | `REPO_PATH` | `repo.path` | `/path/to/repo` |

### 10.3 Full Configuration Reference (`archon.yaml`)

```yaml
# archon.yaml — all values optional if supplied via environment variables
# Environment variable override pattern: ARCHON_<UPPERCASED_DOT_PATH>
# Example: ARCHON_MODE=sandbox, ARCHON_UI_PORT=9090

mode: sandbox               # approval | sandbox

ui:
  port: 8080
  host: "0.0.0.0"          # 127.0.0.1 to restrict to localhost
  auth:
    enabled: false
    username: archon
    password: ""            # ARCHON_UI_AUTH_PASSWORD

jira:
  base_url: ""              # REQUIRED
  email: ""                 # REQUIRED
  api_token: ""             # REQUIRED — or JIRA_API_TOKEN
  poll_interval_seconds: 60
  debounce_seconds: 30
  escalation_timeout_business_days: 5

  projects:
    - key: ENG              # REQUIRED — MVP supports exactly one entry
      watch_filter: |       # JQL — sensible default applied if omitted
        project = ENG
        AND issuetype in (Story, Task, Bug)
        AND status = "Ready for Dev"
        AND assignee is EMPTY
      auto_transition: true  # required in MVP
      transitions:
        in_progress: "In Progress"
        in_review: "In Review"
        done: "Done"

github:
  token: ""                 # REQUIRED — or GITHUB_TOKEN
  repo: ""                  # REQUIRED — owner/repo
  base_branch: main
  draft_prs: false
  labels:
    - archon-generated
  default_reviewers: []

repo:
  path: ""                  # REQUIRED — or REPO_PATH
  primary_language: ""      # auto-detected if omitted

opencode:
  binary_path: opencode     # name or full path
  timeout_minutes: 30
  max_concurrent_sessions: 3
  max_concurrent_evaluations: 2
  max_revision_cycles: 3

sandbox:
  enabled: true             # MVP requires Docker sandbox execution
  image: archon/opencode-sandbox:latest
  cpu_limit: "4"
  memory_limit: 8g
  network: bridge           # bridge | none

state_store:
  path: ~/.archon/archon.db

confidence_threshold: 0.7

log:
  level: info               # debug | info | warn | error
  format: pretty            # pretty | json
```

### 10.4 Docker Compose (Team Deployment)

```yaml
version: "3.9"
services:
  archon:
    image: archon/archon:latest
    ports:
      - "8080:8080"
    volumes:
      - ./archon.yaml:/etc/archon/archon.yaml:ro
      - archon-data:/root/.archon
      - /var/run/docker.sock:/var/run/docker.sock   # for sandbox mode
      - /path/to/your/repo:/workspace:rw
    environment:
      - JIRA_API_TOKEN=${JIRA_API_TOKEN}
      - GITHUB_TOKEN=${GITHUB_TOKEN}
    restart: unless-stopped

volumes:
  archon-data:
```

### 10.5 Desktop Quickstart

```bash
# Install
brew install archon
# or: curl -sSL https://archon.sh/install.sh | sh

# Minimum config via environment
export JIRA_BASE_URL=https://your-org.atlassian.net
export JIRA_EMAIL=you@company.com
export JIRA_API_TOKEN=your-token
export JIRA_PROJECT_KEY=ENG
export GITHUB_TOKEN=ghp_...
export GITHUB_REPO=your-org/your-repo
export REPO_PATH=/path/to/your/repo

archon start
open http://localhost:8080
```

---

## 11. Jira Integration Specification

### 11.1 API
Jira Cloud REST API v3 (`/rest/api/3/`)

### 11.2 Required Permissions
- `read:jira-work` — read issues and comments
- `write:jira-work` — post comments, transition issues

### 11.3 Rate Limiting
- Respect `X-RateLimit-*` and `Retry-After` headers
- Exponential backoff: start 1s, max 60s, ±20% jitter

### 11.4 Comment Authoring
- Authored as the configured service account
- All Archon comments include `[ARCHON]` identifier
- Archon checks comment author against configured email to avoid responding to its own comments
- Each clarification cycle posts a new Archon comment; prior comments are preserved as audit history

---

## 12. opencode Integration Specification

### 12.1 Relationship
Archon uses `opencode` for all AI reasoning. Archon does not call any LLM API directly and has no LLM provider configuration of its own. `opencode` is assumed to be independently configured with its own model and API credentials, baked into the sandbox Docker image or injected at runtime. Archon invokes it as a subprocess inside the sandbox and keeps deterministic git operations outside the agent.

### 12.2 Invocation — Confirmed CLI Contract

Based on current opencode documentation (`opencode.ai/docs/cli`), the correct non-interactive invocation is:

```bash
opencode run \
  --model {provider/model} \
  --format json \
  "{prompt}"
```

Key confirmed facts about the `opencode run` command:

- **`opencode run [message..]`** is the non-interactive subcommand. It accepts the prompt as positional arguments (not via a file or stdin).
- **`--format json`** switches output to raw JSON events rather than formatted text. This is the machine-readable mode Archon uses.
- **`--model provider/model`** (e.g., `anthropic/claude-sonnet-4-20250514`) overrides the configured default model for this invocation. Archon MAY specify a model or leave it unset to use the user's configured default.
- **`--agent`** allows targeting a named opencode agent (see Section 12.5).
- **`--session` / `--continue`** allow resuming a prior session — not used by Archon (each invocation is a fresh session).
- **`--attach http://localhost:4096`** allows connecting to a pre-warmed `opencode serve` instance to avoid cold-boot overhead. Archon SHOULD use this in long-running daemon mode (see Section 12.6).
- There is **no `--prompt-file` flag** and **no `--no-interactive` flag** — the `run` subcommand is inherently non-interactive; the prompt is a positional argument.

Because prompts can be very long (ticket content + rubric + repo context), Archon SHALL use a shell heredoc or a temp file piped through `xargs`/process substitution, or write the prompt to a temp file and pass it via shell substitution:

```bash
# Preferred pattern for long prompts — avoids arg length limits
PROMPT=$(cat /tmp/archon-session-{session_id}.txt)
opencode run --format json "$PROMPT"
```

The temp file is deleted after `opencode` exits.

### 12.3 opencode Configuration for Archon

Archon requires `opencode` to run fully autonomously — no interactive approval prompts, no pauses. This is achieved through opencode's `permission` configuration, which Archon injects via the `OPENCODE_PERMISSION` environment variable (accepted as inline JSON):

```bash
OPENCODE_PERMISSION='{"*":"allow"}' opencode run --format json "$PROMPT"
```

This grants `allow` to all tool calls for the duration of the invocation. In MVP, this is always used inside the Docker sandbox, which is the isolation boundary.

Additionally, Archon SHALL inject the working directory by invoking `opencode run` from the repository root (or specifying it via the `--cwd` global flag if supported). opencode automatically scopes file operations to the working directory it is started in.

### 12.4 opencode Configuration File (in Sandbox Image)

The Archon sandbox image SHALL ship a baked-in `opencode.json` at `/root/.config/opencode/opencode.json` that configures sensible defaults for automated operation:

```json
{
  "$schema": "https://opencode.ai/config.json",
  "autoupdate": false,
  "permission": {
    "*": "allow"
  },
  "provider": {
    "anthropic": {
      "options": {
        "timeout": 600000,
        "chunkTimeout": 60000
      }
    }
  }
}
```

The model and API key are injected at runtime via environment variables (`ANTHROPIC_API_KEY`, etc.) — the sandbox image does not bake in credentials.

### 12.5 Archon-Specific opencode Agent (Recommended Pattern)

Rather than passing the full evaluation and implementation prompts as raw CLI arguments, Archon SHOULD install a custom opencode agent definition into the project's `.opencode/agents/` directory (or into the sandbox image). This agent sets a system prompt that primes opencode for Archon's structured JSON output contract:

`.opencode/agents/archon-eval.md`:
```markdown
---
description: Archon readiness evaluator — returns structured JSON
mode: subagent
permission:
  "*": "allow"
---

You are Archon's requirements evaluator. You will be given a Jira ticket and a readiness rubric.

Evaluate whether the ticket contains sufficient information to implement autonomously.

You MUST respond with ONLY valid JSON matching this exact schema — no preamble, no markdown fences:
{
  "decision": "READY" | "NOT_READY",
  "confidence": <float 0.0-1.0>,
  ...
}
```

`.opencode/agents/archon-impl.md`:
```markdown
---
description: Archon implementation agent — implements ticket, outputs JSON summary
mode: subagent
permission:
  "*": "allow"
---

You are Archon's implementation agent. You will be given a Jira ticket and implementation instructions.

Do not create branches, commit, or push. Archon performs deterministic git operations outside the agent.

Implement the changes, then respond with ONLY valid JSON matching this schema:
{
  "task": "implementation",
  "status": "success" | "failure",
  ...
}
```

Invocation using the named agent:
```bash
opencode run --agent archon-eval --format json "$PROMPT"
opencode run --agent archon-impl --format json "$PROMPT"
```

This approach has two advantages: it keeps system prompt logic out of Archon's Go code (where it is harder to iterate on), and it allows users to customize the agent behavior by editing the markdown files in their repo.

### 12.6 Long-Running Server Mode (Performance Optimization)

Each cold `opencode run` invocation starts a new process, loads MCP servers, and initializes the provider connection — adding several seconds of overhead per ticket. For high-throughput team deployments, Archon SHOULD use `opencode serve` to pre-warm a persistent backend and attach each invocation to it:

```bash
# Archon starts this once on daemon startup
opencode serve --port 4096 &

# Each session invocation attaches to the warm server
opencode run --attach http://localhost:4096 --agent archon-eval --format json "$PROMPT"
```

The serve instance is tied to the repository working directory. For monorepos, a single serve instance covers the full codebase. Archon SHALL restart the serve instance if it exits unexpectedly.

This is a non-blocking optimization; Archon SHALL fall back to cold invocation if the serve instance is unavailable.

### 12.7 Output Parsing

With `--format json`, opencode streams a series of newline-delimited JSON event objects to stdout. Archon SHALL:

1. Buffer all stdout until the process exits
2. Find the last complete JSON object in the buffer (the final response)
3. Parse it and validate it against the expected schema for the task type (evaluation or implementation)

The exact shape of the JSON event stream from `--format json` should be treated as the source of truth from the opencode docs/server spec, and Archon's parser should be tolerant of additional fields it does not recognize.

### 12.8 Failure Modes

| Condition | Archon Response |
|---|---|
| Exit 0, valid JSON | Process result normally |
| Exit 0, malformed or missing JSON | Retry once with explicit JSON output instruction appended to prompt; if still malformed → mark EVALUATION_ERROR |
| Non-zero exit | Mark FAILED; capture stderr; post Jira comment with excerpt |
| Timeout (30 min default) | Kill process/container; mark FAILED; post Jira comment |
| Verification command fails after `opencode` exits | Mark FAILED; do not commit, push, or open a PR; post Jira comment with the failing command excerpt |
| serve instance down | Fall back to cold invocation; log warning |

---

## 13. Execution Sandbox Specification

### 13.1 Sandbox Image (`archon/opencode-sandbox`)
The maintained image includes:
- `opencode` (version-pinned)
- Node.js LTS, Python 3.x, Go latest stable
- Build tools: `make`, `git`, `curl`, standard POSIX utilities
- Package managers: `npm`, `pnpm`, `yarn`, `pip`, `uv`, `go`

Users may substitute a custom image via `sandbox.image`.

### 13.2 Container Lifecycle
1. Pull image if not cached locally
2. Archon creates or refreshes the isolated session worktree on the host
3. Create container with the session worktree volume and injected environment variables
4. Invoke `opencode` inside container
5. Stream logs to Archon in real-time
6. After `opencode` exits, Archon runs verification, commit, and push steps against the same worktree
7. Destroy container after exit (pass or fail)

### 13.3 Volume Strategy
- Isolated session worktree mounted read-write at `/workspace`
- Archon performs git operations (worktree create, branch create, commit, push) on the host against that same worktree
- Archon monitors GitHub for the expected branch and PR after container exits

### 13.4 Security Posture
- No privileged mode required
- Non-root container user
- Host filesystem access limited to repository mount and a temp directory
- Capabilities dropped to minimum

---

## 14. Readiness Rubric — Database Storage

### 14.1 Storage Location
The readiness rubric lives in Archon's SQLite database. It is editable at runtime via the web UI Settings panel without restarting the daemon.

### 14.2 Default Rubric (Seeded on First Run)

| Criterion ID | Description | Applies To |
|---|---|---|
| `clear_goal` | The problem or objective is unambiguously stated | All |
| `testable_acceptance_criteria` | Acceptance criteria exist and are specific enough to write a test | All |
| `scope_defined` | In-scope and out-of-scope behavior is explicitly or inferably defined | All |
| `dependencies_named` | All referenced systems, APIs, or data models are named | All |
| `steps_to_reproduce` | Steps to reproduce are provided | Bug only |
| `expected_vs_actual` | Expected and actual behavior are described | Bug only |
| `environment_specified` | The environment where the bug occurs is specified | Bug only |
| `research_question` | A specific research question is stated | Spike only |
| `defined_deliverable` | A defined deliverable exists (doc, decision, or PoC) | Spike only |

### 14.3 Rubric Schema

```sql
CREATE TABLE rubric_criteria (
  id          TEXT PRIMARY KEY,
  description TEXT    NOT NULL,
  applies_to  TEXT    NOT NULL DEFAULT 'all',  -- 'all', 'Bug', 'Spike', etc.
  is_required INTEGER NOT NULL DEFAULT 1,
  project_key TEXT,   -- NULL = applies to all projects
  is_enabled  INTEGER NOT NULL DEFAULT 1,
  created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
  updated_at  DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### 14.4 Runtime Editing
Via the web UI Settings panel, authorized users can:
- Enable / disable individual criteria
- Edit criterion descriptions
- Add custom criteria
- Scope criteria to a specific project vs. globally

Changes take effect on the next evaluation.

---

## 15. Security & Compliance

### 15.1 Credential Management
- Secrets stored as environment variables or config file entries
- Archon warns on startup if `archon.yaml` is detected inside a `.git`-tracked directory
- Secrets redacted in all log output and UI displays

### 15.2 Least Privilege
- Jira account: `read:jira-work` + `write:jira-work` only
- GitHub token: `contents: write`, `pull_requests: write` (fine-grained PAT preferred)
- `opencode` inherits only credentials explicitly injected via environment

### 15.3 Data Handling
- Ticket content is passed to `opencode`, which may transmit it to an LLM provider. Operators are responsible for ensuring ticket content is appropriate for their `opencode` configuration.
- `sensitive_fields` config option strips named Jira custom fields from all `opencode` prompts

### 15.4 Audit Trail
- Every state transition, `opencode` invocation (prompt hash), and GitHub write is logged to the state store with timestamp and actor
- Viewable per ticket in the web UI and via `archon audit <issue-key>`

---

## 16. Observability & Monitoring

### 16.1 Logging
- Structured logs: `level`, `timestamp`, `component`, `session_id`, `issue_key`
- Default format: `pretty` (desktop); `json` (Docker/CI)

### 16.2 Prometheus Metrics (`/metrics`)

| Metric | Type | Description |
|---|---|---|
| `archon_tickets_observed_total` | Counter | Tickets observed |
| `archon_evaluations_total` | Counter | Evaluations (labels: `decision`, `project`) |
| `archon_clarification_comments_total` | Counter | Clarification threads posted |
| `archon_approvals_total` | Counter | Approvals (label: `outcome=approved\|rejected`) |
| `archon_opencode_sessions_total` | Counter | opencode sessions spawned (label: `task`) |
| `archon_opencode_sessions_success_total` | Counter | Successful sessions |
| `archon_opencode_sessions_failed_total` | Counter | Failed sessions |
| `archon_prs_opened_total` | Counter | PRs opened |
| `archon_prs_merged_total` | Counter | PRs merged |
| `archon_sessions_active` | Gauge | Active sessions |
| `archon_evaluation_duration_seconds` | Histogram | Evaluation latency |
| `archon_implementation_duration_seconds` | Histogram | Implementation latency |

### 16.3 Health Endpoints
- `GET /health` — 200 if Jira and GitHub are reachable
- `GET /health/ready` — 200 if actively watching

---

## 17. Error Handling & Recovery

### 17.1 Jira API Failures
- Transient: exponential backoff, up to 5 retries; skip poll cycle if all fail
- Auth (401): halt processing; display error in UI

### 17.2 opencode Failures
- Evaluation failure: mark `EVALUATION_ERROR`; retry on next poll cycle; do not post Jira comment
- Implementation failure: mark `FAILED`; post Jira comment with summary and stderr
- Timeout: kill container/process; mark `FAILED`; post Jira comment

### 17.3 GitHub API Failures
- PR creation failure: retry up to 3 times; if still failing → mark `PR_CREATION_FAILED`; post Jira comment with branch name for manual PR creation

### 17.4 State Store
- Schema validation on startup; refuse to start on mismatch with clear migration instructions
- SQLite WAL mode enabled for crash safety

### 17.5 Daemon Recovery on Restart
- Sessions in `IMPLEMENTING` state at restart are moved to `FAILED` (the process is no longer running)
- Operator can retry from the web UI or CLI

---

## 18. CLI Interface

### 18.1 Commands

```
archon start                              Start daemon (and web UI)
archon stop                               Graceful stop
archon status                             Daemon status and active sessions

archon session list [--state <s>] [--project <k>]
archon session show <issue-key>
archon session retry <issue-key>          Force re-evaluation
archon session approve <issue-key>        Approve (approval mode)
archon session reject <issue-key> --reason "..."
archon session abandon <issue-key>

archon audit <issue-key>                  Full audit trail

archon rubric list                        Show readiness rubric
archon rubric edit                        Interactive rubric editor

archon config validate                    Validate archon.yaml
archon config show                        Show resolved config (secrets redacted)

archon version
```

### 18.2 Output Formats
All commands support `--output json`.

In MVP, the CLI is an operational companion to the mandatory web UI rather than a full replacement for it.

---

## 19. Implementation Checklist

Ordered by dependency. Each group should be substantially complete before the next group depends on it.

### Group 1 — Foundation
- [ ] Project scaffold (Go, module layout, build tooling)
- [ ] Config loader (YAML file + environment variable overrides)
- [ ] SQLite state store with schema, migrations, WAL mode
- [ ] Structured logging
- [ ] ADF-to-plaintext parser

### Group 2 — Jira Integration
- [ ] Jira REST client (auth, search with JQL, get issue, post comment, get transitions, transition issue)
- [ ] Polling loop with debounce and updated-timestamp high-water mark
- [ ] Ticket exclusion logic (existing branch detection on GitHub via `archon/{issue-key}-*`, `archon-skip` label)
- [ ] Ticket normalization to structured plain text

### Group 3 — opencode Integration
- [ ] opencode subprocess invocation wrapper (prompt file, workdir, timeout, log streaming)
- [ ] Evaluation prompt template (ticket + rubric + prior context)
- [ ] Implementation prompt template (ticket + analysis + repo context)
- [ ] JSON response parser and validator
- [ ] Retry logic on malformed JSON

### Group 4 — Core State Machine
- [ ] Session model and all state transitions (write-ahead persistence)
- [ ] Evaluator: invoke opencode, parse result, route READY / NOT_READY
- [ ] Default readiness rubric seeded to database on first run
- [ ] Clarification loop: post comment with suggested answers, enter waiting state, re-evaluate on update

### Group 5 — Monorepo Context
- [ ] Repository structure scanner (top-level dirs, workspace manifests, package manager detection)
- [ ] Repo context block included in all opencode prompts

### Group 6 — Approval Gate
- [ ] `AWAITING_APPROVAL` state and transitions
- [ ] Approval notification Jira comment with UI link
- [ ] Approve / Reject / Edit-scope-then-approve API endpoints

### Group 7 — Execution Sandbox
- [ ] Docker availability detection at startup
- [ ] Session worktree lifecycle management (create, reuse-from-branch-head for revisions, cleanup)
- [ ] Container lifecycle management (mount worktree, run, stream logs, destroy)
- [ ] Sandbox image definition (`archon/opencode-sandbox`)

### Group 8 — GitHub Integration
- [ ] GitHub REST client (create PR, list PRs, poll review comments, add label)
- [ ] PR creation with full body template
- [ ] PR review comment polling and revision trigger
- [ ] Verification gate: block commit/push/PR creation on failed tests

### Group 9 — Web UI
- [ ] Embedded HTTP server (static assets + REST/WebSocket API)
- [ ] Kanban board layout with column mapping
- [ ] Ticket card (state, metadata, quick actions)
- [ ] Card detail side panel (evaluation result, audit trail, live log)
- [ ] Approval / Reject / Edit-scope controls
- [ ] Live log streaming (WebSocket or SSE)
- [ ] Settings panel with rubric editor
- [ ] First-run setup wizard
- [ ] Optional basic authentication

### Group 10 — CLI
- [ ] All commands in Section 18.1
- [ ] `--output json` for all commands

### Group 11 — Observability & Reliability
- [ ] Prometheus metrics endpoint
- [ ] Health endpoints
- [ ] Exponential backoff for all external calls
- [ ] Graceful shutdown with session drain
- [ ] Daemon restart recovery (IMPLEMENTING → FAILED)
- [ ] Persist audit trail for per-cycle clarification comments and worktree-backed revision runs

### Group 12 — Distribution
- [ ] Single binary build (macOS arm64/amd64, Linux amd64/arm64)
- [ ] Docker image with opencode pre-installed
- [ ] `docker-compose.yml` reference
- [ ] README with quickstart

---

## 20. Resolved Design Questions

All open questions from prior drafts have been resolved. Decisions are recorded here for traceability.

---

**Q1: What is the exact `opencode` CLI interface for non-interactive invocation?**

**Resolved — researched against opencode docs (opencode.ai/docs/cli, April 2026).**

The correct subcommand is `opencode run [message..]`. Key facts:
- The prompt is a positional argument, not a file or stdin. For long prompts, use shell variable expansion (`"$PROMPT"`) from a temp file.
- `--format json` is the flag for machine-readable output (not `--output-format`).
- There is no `--no-interactive` or `--prompt-file` flag — `run` is inherently non-interactive.
- `--agent` selects a named agent, enabling Archon-specific system prompts stored as markdown agent files.
- `--attach http://localhost:4096` connects to a pre-warmed `opencode serve` instance for lower-latency repeated invocations.
- Permissions for autonomous operation are injected via the `OPENCODE_PERMISSION` environment variable as inline JSON (`'{"*":"allow"}'`).

Full integration specification is in Section 12.

---

**Q2: Does `opencode` need the full repo mounted for suggested answers, or can file excerpts be passed in the prompt?**

**Decision: Full repo working directory, always.**

opencode's built-in tools (`read`, `grep`, `glob`, `bash`) allow it to actively navigate and search the codebase as part of answering the evaluation prompt. This is fundamentally more powerful than pre-selecting files to include in the prompt, because Archon cannot know in advance which files are relevant to an arbitrary ticket. Passing excerpts would require Archon to do the very search that opencode is better equipped to perform.

In practice: Archon invokes `opencode run` from the repository root (or with the repo root as the working directory). opencode's default permissions allow `read` and `grep` across the working directory. The evaluation prompt instructs opencode to search the codebase when formulating suggested answers. No additional repo context injection is needed in the prompt beyond the directory map described in Section 8.

Latency concern is acceptable — evaluation only runs when a ticket changes, not on a hot path.

---

**Q3: How should Archon handle a ticket updated mid-implementation?**

**Decision: Ignore during implementation; flag for re-evaluation on completion.**

When a session is in `IMPLEMENTING` or `REVISING` state, Archon SHALL:
- Continue the current `opencode` session to completion uninterrupted
- Record the detected update in the state store with a timestamp
- On session completion (success or failure), check whether the ticket was updated during the run
- If updated: post a Jira comment noting that the ticket changed during implementation and that Archon will re-evaluate the completed work against the new description
- Move the session back to `EVALUATING` if the PR is not yet open, or flag the PR with an `archon-changed-during-impl` label if it is already open

Aborting mid-implementation on description changes would waste significant compute and produce confusing partial branches. Completing and flagging is the right tradeoff.

---

**Q4: Should the web UI prevent conflicts when two users approve the same session simultaneously?**

**Decision: Optimistic locking — lightweight and sufficient for MVP team sizes.**

The `sessions` table in SQLite will include a `version` integer column incremented on every state write. The approve endpoint performs a conditional update:

```sql
UPDATE sessions
SET state = 'IMPLEMENTING', version = version + 1
WHERE id = ? AND state = 'AWAITING_APPROVAL' AND version = ?
```

If the update affects 0 rows, the approval was a conflict (another user already approved or the state changed). The UI receives a 409 and shows a "this session was already actioned" message. No distributed locking infrastructure needed.

---

**Q5: Should Archon build and publish the sandbox Docker image, or should users supply their own?**

**Decision: Archon maintains and publishes an official image; users can override.**

`archon/opencode-sandbox:latest` is built and published by this project as part of the release pipeline. The image is tagged by opencode version (e.g., `archon/opencode-sandbox:opencode-0.1.50`) so Archon's release notes can specify the tested pairing. Users override via `sandbox.image` in config.

Rationale: zero-friction startup is a core goal. Requiring users to build or supply their own image contradicts that. The image is small (opencode + language runtimes) and straightforward to maintain. The build pipeline is a one-time setup cost.

---

**Q6: Is Docker sandbox execution required in MVP?**

**Decision: Yes. Docker sandbox execution is required in MVP.**

Archon SHALL run all implementation and revision cycles inside the Docker sandbox. MVP does not include a host-execution fallback.

---

**Q7: Is the web UI mandatory in MVP?**

**Decision: Yes. The web UI is mandatory in MVP.**

The UI is the primary operating surface for session visibility, approvals, and overrides. The CLI is additive and does not replace the UI in MVP.

---

**Q8: Who owns deterministic git operations?**

**Decision: Archon owns them.**

Archon creates worktrees and branches, runs verification commands, creates commits, and pushes branches. `opencode` edits code and returns structured results but does not perform branch creation, commit, or push.

---

**Q9: What happens if tests fail?**

**Decision: Fail closed.**

If required tests or verification commands fail, Archon SHALL mark the session `FAILED` and SHALL NOT commit, push, or open a PR.

---

**Q10: What is the primary MVP deployment target?**

**Decision: Local solo developer.**

MVP is optimized for one developer running Archon locally against one Jira project and one GitHub repository. Team deployment remains supported by the architecture but is not the primary MVP target.

---

**Q11: How rich must suggested answers be in MVP?**

**Decision: Repo-aware prose is sufficient.**

Suggested answers should clearly reflect repository knowledge, but exact file and line citations are optional rather than required.

---

**Q12: How should Archon detect existing work for a ticket in MVP?**

**Decision: Branch naming convention only.**

For MVP, `archon/{issue-key}-*` branches are the sole linkage mechanism used to detect that a ticket already has work in progress or has already been implemented.

---

**Q13: Should clarification threads update an existing comment or create a new one?**

**Decision: Create a new comment per cycle.**

Each clarification cycle posts a new Jira comment so the conversation history remains intact and auditable.

---

**Q14: How is repository isolation handled per run?**

**Decision: Fresh git worktree every time.**

Every implementation and revision cycle gets its own isolated worktree. First implementations branch from the configured base branch; revisions use a fresh worktree from the current Archon branch head.

---

**Q15: Is the revision loop in scope for MVP?**

**Decision: Yes.**

The `archon-revise` workflow, including fresh-worktree revision runs on the same branch, is part of MVP.

---

**Q16: Are Jira status transitions required in MVP?**

**Decision: Yes.**

Archon SHALL perform configured transitions for implementation start, PR open, and PR merge whenever those transitions are valid from the current Jira status.

---

**Q17: How many Jira projects and repositories are in MVP scope?**

**Decision: One Jira project and one GitHub repository.**

MVP supports a single watched Jira project and a single target GitHub repository.

---

## 21. Appendix

### 21.1 Glossary

| Term | Definition |
|---|---|
| ADF | Atlassian Document Format — Jira's JSON-based rich text format |
| Approval Mode | Operating mode requiring human sign-off before each implementation |
| Execution Sandbox | Docker container in which `opencode` runs |
| JQL | Jira Query Language |
| opencode | The AI coding agent Archon uses for all AI tasks |
| Rubric | Readiness criteria stored in Archon's SQLite database |
| Sandbox Mode | Fully autonomous mode using the Docker execution sandbox |
| Session | The complete Archon lifecycle for a single Jira ticket |
| State Store | Archon's SQLite database |
| Suggested Answer | A repo-informed candidate answer to a clarification question |

### 21.2 Related References
- opencode documentation and CLI reference
- Jira REST API v3: `https://developer.atlassian.com/cloud/jira/platform/rest/v3/`
- GitHub REST API: `https://docs.github.com/en/rest`
- Atlassian Document Format specification

### 21.3 Revision History

| Version | Date | Changes |
|---|---|---|
| 1.0 | 2026-04-02 | Initial draft |
| 1.1 | 2026-04-02 | opencode as sole AI engine (no separate LLM config); web UI Kanban board; human-gated approval mode; Docker execution sandbox; zero-config startup (7 required values); monorepo-first; rubric in database; GitHub-only; repo-informed suggested answers; pre-existing PRs excluded from MVP; time estimates removed; sandbox mode replaces dry-run; open questions resolved or updated |
| 1.2 | 2026-04-02 | Section 12 fully rewritten with researched opencode CLI contract (`opencode run`, `--format json`, `OPENCODE_PERMISSION` env var, `--agent`, `--attach` serve mode, no `--prompt-file`/`--no-interactive`); Archon agent markdown pattern introduced; all 5 open questions resolved with firm decisions; section renamed to "Resolved Design Questions" |
| 1.3 | 2026-04-03 | MVP scope decisions locked: Docker sandbox required; web UI mandatory; Archon owns git operations; test failures block commit/push/PR creation; solo-local deployment target; suggested answers may be repo-aware prose; branch-convention-only ticket linkage; new clarification comment per cycle; fresh git worktree per implementation and revision cycle; revision loop in scope; Jira transitions required; MVP limited to one Jira project and one GitHub repository |
