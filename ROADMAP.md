# ROADMAP — hardware-usage

What exists, what is missing, and what is deliberately out of scope. Details: `IDEA.md`
(intent), `task_plan.md` (executable checklist), `findings.md` (decisions and discoveries).

## What exists today

- A live, local-only web page (`127.0.0.1:8765`) served by a Go `net/http` server with an
  embedded static frontend (vanilla JS, no build step), pushed over SSE (~2s cadence).
- System header: whole-machine CPU, RAM, disk and network I/O rates, temperature, AMD GPU
  (busy %, VRAM, temperature via sysfs).
- Per-process list (CPU, RAM, disk I/O), sorted heaviest-first (RAM by default, toggle to
  CPU or disk I/O), with a derived identity label (e.g. Chromium child process roles).
- Docker containers grouped by their compose stack (`com.docker.compose.project`), with
  per-container CPU and RAM served from a background-refreshed cache.
- Actions behind a confirmation: kill (SIGTERM) per process, stop/restart per container.
- Every metric is absent-tolerant: missing sensors, denied `/proc/<pid>/io`, or an
  unreachable Docker daemon degrade to absent fields, never a crash.
- `bin/ci` gate (gofmt, vet, golangci-lint, test, govulncheck) — the same script CI runs.

## What is missing / natural next steps

- Process detail panel: clicking a row shows everything `/proc` exposes for that pid
  (full command line, cwd, exe, threads, state, parent, fds, …), fetched on demand.
- Decision on Chromium remote debugging (CDP) for tab-level identity: `/proc` alone can
  only name a browser child's role, never the tab; the trade-offs (security surface,
  setup cost, browser coupling) are to be weighed in writing before any build.
- History (time-series, charts): explicitly deferred to a possible v2 — it requires a
  background collector and storage, which v1 rules out. Build only if the need proves real.

## Deliberately out of scope

- Not a monitoring/observability platform (no Grafana, no Prometheus, no exporters).
- No history in v1: no background collector, no storage, no time-series charts.
- Single machine, single user, loopback only — no remote hosts, no multi-user, no auth.
- Not a 24/7 daemon: collection runs only while the page is open (client-gated ticker).
- Per-process GPU and network metrics: system-level only in v1.
