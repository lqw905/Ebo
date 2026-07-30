# Ebo

Ebo is a local CLI runtime for managing a project Prompt Tree. It does not call AI APIs, does not generate prompts, and does not execute code changes by itself. Humans or external agents create Prompt Markdown; Ebo stages, reviews, approves, applies, validates, and scans the tree.

This repository currently contains the first MVP slice.

## Project Isolation

`ebo init` keeps project intent under one `.ebo/` namespace instead of scattering Prompt files through application source directories:

```text
.ebo/
  WORKFLOW.md 中文使用手册和可复制的 Agent 提示词
  tree/       canonical, version-controlled Prompt Tree
  proposals/  Prompt changes waiting for human review
  plans/      execution plans
  receipts/   execution evidence
  runtime/    generated context and imports
  cache/      ignored local cache
  locks/      ignored process locks
  tmp/        ignored temporary files
```

It also generates a Chinese `.ebo/WORKFLOW.md` with copyable prompts for each user and agent flow. A small managed block in `AGENTS.md` and, when requested, `CLAUDE.md`, tells the agent to read that document before working. Only Prompt proposals that pass hash-bound human approval can enter the tree as executable work.

Ebo owns the execution decision so an agent does not need to load the whole Prompt Tree. `project.root`, unapproved prompts, already-satisfied hashes, and prompts with unready dependencies are skipped. Failed or blocked prompts can be retried. The agent loads only the selected Prompt and its direct semantic links with `ebo context <prompt-id> --depth 0` after `ebo next` returns one executable task. Broader hierarchy context is opt-in, which keeps unnecessary work and context usage low.

## Build

```bash
go test ./...
go build -o ebo ./cmd/ebo
```

## CI

GitHub Actions runs on Windows, macOS, and Linux:

```text
go test ./...
go build ./cmd/ebo
go test ./internal/cli -run TestCLISmoke -count=1
node scripts/check-npm-packages.mjs
npm pack --dry-run ./npm/root
```

The npm CI job also runs a packaging smoke test with fake binaries to verify the supported platform package layout.

## Implemented Commands

```bash
ebo init --agents codex,claude
ebo doctor
ebo status
ebo config get

ebo add --stdin
ebo add --file <path>
ebo add --dir <path>
ebo add --dry-run --file <path>
ebo review [proposal-id]
ebo approve <proposal-id>
ebo reject <proposal-id> --reason "..."
ebo apply <proposal-id>

ebo tree list
ebo tree show <node-id>
ebo tree validate
ebo tree search "<text>"
ebo tree graph [node-id]
ebo tree graph --around <node-id>
ebo context <node-id> --depth 2 --out .ebo/runtime/context.json

ebo scan [node-id]
ebo plan [node-id]
ebo plan list
ebo plan show <plan-id>
ebo next [plan-id]
ebo export <plan-id> --format markdown
ebo export <plan-id> --format json
ebo report <task-id> --plan <plan-id> --result passed --note "..."
ebo verify <plan-id>
ebo abort <plan-id>
ebo commit <plan-id> --dry-run
ebo import . --out .ebo/runtime/import
ebo lock status
ebo guard check
ebo guard check --staged
ebo hook pre-write --path <file>
ebo hook pre-write --path <file> --json
ebo hooks install
ebo hooks status
```

`approve` intentionally requires an interactive terminal and a `[y/N]` confirmation. Ebo binds the full proposal hash internally and verifies it again during `apply`, so users do not need to copy a SHA-256 value while changed content is still rejected.

## Current Scope

- `.ebo/` project layout and root prompt initialization.
- Generated Chinese `.ebo/WORKFLOW.md` with copyable prompts for every common Ebo flow.
- Managed Ebo blocks in `AGENTS.md` and `CLAUDE.md`.
- Markdown Prompt parsing with YAML Front Matter subset support.
- Proposal creation from stdin, file, or directory.
- Hash-bound interactive approval.
- Apply via a validated temporary candidate tree.
- Single-root tree validation, parent checks, link target checks, and `depends_on` cycle checks.
- Stable content and effective hashes.
- Deterministic dirty-node scan.
- Persistent execution plans under `.ebo/plans/`.
- Plan-based `next`, `export`, `report`, `verify`, and `abort`.
- `report passed|failed|blocked` writes execution state back to `.ebo/tree/` when the plan hashes still match.
- Project-level lock file at `.ebo/locks/project.lock` for mutating commands.
- Fail-closed execution gate with one ignored `.ebo/runtime/active-task.json` lease.
- Git baseline enforcement before planning or execution.
- `ebo guard check` for working-tree authorization and staged-plan validation.
- Strict Agent pre-write decisions with deterministic exit codes and optional JSON output.
- Optional Prompt `scope.allow` and `scope.deny` globs for file-level write authorization.
- Optional managed pre-commit hook installed with `ebo hooks install`.
- Conservative `commit` orchestration for completed plans.
- Basic evidence package export for reverse import workflows.
- npm root launcher and platform package skeletons under `npm/`.
- Initial npm distribution targets Windows x64 and macOS arm64.

## Not Complete Yet

- Full Cobra-based command layer.
- Full YAML 1.2 parser integration and JSON Schema validation.
- Automatic staging of implementation code for one plan per commit.
- Automatic installation of Agent-specific native pre-write adapters; Agent runtimes can call `ebo hook pre-write` today.
- npm Trusted Publishing must be configured on npm before the release workflow can publish packages.

## Release

Tagging `vX.Y.Z` triggers `.github/workflows/release.yml`.

The release workflow:

```text
go test ./...
build windows/amd64 and darwin/arm64 binaries
write dist/checksums.txt
node scripts/prepare-npm-packages.mjs X.Y.Z dist
npm pack --dry-run for every package
npm publish platform packages first
npm publish @aibo666/ebo last
create GitHub Release with binaries and checksums
```

The first npm release intentionally ships only:

```text
@aibo666/ebo
@aibo666/ebo-win32-x64
@aibo666/ebo-darwin-arm64
```

Other platforms can be added when there is demand.

## Prompt Boundary

Ebo accepts prompt content only through explicit commands:

```bash
ebo add --stdin
ebo add --file drafts/feature.md
ebo add --dir drafts/prompts
```

Those commands create proposals only. They never write directly to `.ebo/tree/`.
