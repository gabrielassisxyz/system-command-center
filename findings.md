# findings.md — hardware-usage

> Durable memory across loop iterations. Each fresh agent reads this first.
> Start with the load-bearing decisions; iterations append discoveries below.

## Decisions (from IDEA.md / SCOPE.md)
- WHAT: live, local-only web page showing this desktop's resource usage — a per-process
  list + Docker containers grouped by their compose stack, pushed live to the browser,
  sorted by a chosen metric (RAM default).
- Anti-goal: NOT a monitoring/observability platform, NOT multi-user, NOT a 24/7 daemon.
  v1 keeps NO history — no background collector, no storage, no time-series (deferred v2).
- Stack / storage: **Go 1.26** (stdlib `net/http` + SSE), `gopsutil` for system/process
  metrics, official `docker/docker` client grouped by `com.docker.compose.project`.
  Frontend: one static HTML page + vanilla JS, no build step. Storage: none (live-only).
  Server binds loopback only. Tier 2.
- Done-check: open the site, see live metrics of this desktop sorted by highest usage of
  the chosen metric (RAM default) — both the per-program list AND Docker containers
  grouped by compose stack, each showing its own usage.

## Discoveries (appended by the loop)
