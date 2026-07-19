# AGENTS.md — hardware-usage

Live, local-only web page showing this desktop's resource usage: a per-process list and Docker containers grouped by their compose stack, pushed live to the browser.

Current scope: read system + per-process metrics and Docker container stats on demand while the page is open, group containers by `com.docker.compose.project`, push to a static browser page over SSE. No history, no storage, no background collector; single machine (this omarchy desktop), single user. Don't expand beyond it without a present need; if a change drifts past it, STOP and flag it. Full intent: see `IDEA.md`.

## Stack

- **Go 1.26**, stdlib `net/http` for the server + SSE (no web framework).
- Metrics: `gopsutil` (system + per-process). Docker: official `docker/docker` client, group containers by the `com.docker.compose.project` label.
- Frontend: one static HTML page + vanilla JS, SSE for live push. No build step.
- Server binds **loopback only** (`127.0.0.1`) — a local tool, keep it that way.

## Commands

- `bin/dev` — run the server locally (http://127.0.0.1:8765).
- `bin/ci` — the one gate: gofmt -> vet -> golangci-lint -> test -> govulncheck. **CI runs this exact script.** Green locally == green in CI. Run it before every push.
- `bin/install-hooks` — one-time after clone: installs the gitleaks pre-commit hook.

## Gate (deterministic — never bypass)

- **Pre-commit**: gitleaks scans staged changes. If it blocks, a secret is staged — fix it, never `git commit --no-verify`.
- **`bin/ci` must be green before push** — it is the same script CI runs on `main`.

## Working rules

- Default branch is **main**. Never commit to it directly: branch (`feature/…`), then merge. Work in your own git worktree.
- Tests: `go test ./...` — table tests, `httptest` for handlers. Add a failing repro test first for a bug; keep the suite green before and after a refactor.
- The universal engineering principles (KISS, YAGNI, atomic commits, WHY-not-WHAT comments, make-it-work→right→fast) live in global config and still apply here.

## The build loop (task_plan.md)

This repo is driven by an executable checklist:
- `task_plan.md` — one atomic task per line, each with a `done when:` acceptance check.
- `findings.md` — durable decisions + discoveries; a fresh agent reads this **first**.
- `progress.md` — one line per completed task (the trajectory log).

Implement the top unchecked `task_plan.md` item, then append to `progress.md`.

## Common hurdles

- `gopsutil` temperature/sensor readings can be empty on some kernels — treat a missing metric as absent, don't crash the view.
- Docker stats need read access to the docker socket; run as a user in the `docker` group or container stats come back silently empty.
- Docker `stats?stream=false` blocks ~1-2s per container (it samples CPU over one interval). Fetch container stats concurrently, or the whole live snapshot freezes for seconds×N-containers — the SSE frame only goes out after the full assembly returns. Concurrency alone is not enough: keep Docker off the snapshot path entirely (it is served from a background-refreshed cache) or every frame still pays the full cost.
- Chromium-family processes (`brave`) write their command line **space-separated with no NUL separators**, so `CmdlineSliceWithContext` returns the whole line in one slice element while normal programs come back split. Split each element on whitespace before scanning for flags — and write the fixture in that single-element shape, or the test passes against a form production never produces.
- `/proc/<pid>/cwd` for a sandboxed Chromium child is `/proc/<pid>/fdinfo`, not a real directory; treat `/proc/*`, `/`, and `$HOME` as no working directory at all.
- (living section — append a line on every gotcha you hit)
