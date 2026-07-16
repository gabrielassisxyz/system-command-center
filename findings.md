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
- v1 also includes (confirmed additions, see `SCOPE.md`): a **system-summary header**
  (whole-machine CPU, RAM, disk & network I/O, temperature, AMD GPU), **per-process disk
  I/O**, and **actions** — kill for processes (SIGTERM), stop/restart for containers.

## Architecture (assumed by task_plan.md)

- **Packages:** `internal/model` (Snapshot structs, the backend↔frontend contract),
  `internal/metrics` (system + per-process collectors), `internal/gpu` (AMD sysfs reader),
  `internal/docker` (container stats + compose grouping), one assembler that builds a
  Snapshot, `internal/server` (http: static page, SSE, action endpoints), `cmd/<bin>`
  wires it. Static page embedded via `embed`.
- **Testability — wrap third-party libs behind thin interfaces this project owns**
  (`SystemSource`, `ProcessSource`, `DockerSource`, a GPU sysfs root, an io-counter
  source). Collectors take these as dependencies so tests inject fakes; no test touches
  real hardware or the docker socket. This is the load-bearing reason collectors are split
  the way they are in the plan.
- **Rates need state.** Disk/network I/O and per-process disk I/O are cumulative counters;
  rate = delta / deltaT across consecutive samples. The assembler is **stateful** — it
  holds the previous sample and an injectable clock; the first sample yields zero rates.
- **Collection is client-gated.** The SSE hub owns the ~2s ticker and only ticks (and thus
  only collects) while ≥1 subscriber is connected — this is what keeps "nothing runs in the
  background" true.
- **Absent-tolerant everywhere.** Any missing metric (temps on some kernels, GPU files,
  denied `/proc/<pid>/io`, docker down) is represented as absent in the model, never a
  crash and never a fatal error for the whole snapshot.
- **Metric placement (from the cost analysis):** per-process/per-container rows carry only
  the metrics that are reliable per-pid — CPU, RAM, disk I/O. Temperature, network, and GPU
  are system-level only and live in the header (per-process GPU/network are out of v1).
- **Per-process disk I/O needs root** (`/proc/<pid>/io` is owner/privileged-only); run the
  server with `sudo` for complete coverage, otherwise it's own-processes-only.

## Discoveries (appended by the loop)
- SSE hub testability: production uses `SSEHub.Run()` with a real 2s ticker, but the hub exposes a manual `Tick()` method and `NewMuxWithHub()` so tests can drive broadcast frames deterministically without waiting on real time. `EmptyProvider` is the temporary stub until the assembler is wired.
- `gopsutil` v4 split temperature sensors into a separate `sensors` package (`github.com/shirou/gopsutil/v4/sensors`) rather than `host.SensorsTemperatures()` from v3. The `SystemSource` interface uses `sensors.TemperatureStat`.
- `cpu.PercentWithContext(ctx, 0, false)` with interval 0 uses gopsutil's internally cached previous sample, so the very first call on a fresh process may return 0; subsequent ticks while the server is running produce meaningful deltas.
- System-level collection is intentionally absent-tolerant: `SystemCollector.Collect` returns a snapshot even if CPU, RAM, or temperature calls error. A source error only leaves that metric field empty.
- Per-process disk I/O is implemented behind a project-owned `ProcIOSource` interface (`internal/metrics/procdisk.go`). `ProcessDiskIOCollector.Collect` takes the current pid list and returns a `map[int32]model.DiskIORate`; denied/missing reads are silently dropped per pid. Rate = delta / deltaT across consecutive samples; first sample yields absent rates. Counter resets (cur < prev) are guarded by returning absent for that direction so the UI never shows negative rates.
- AMD GPU discovery is not hard-coded to `card0`; scan `/sys/class/drm/card*/device/vendor` for `0x1002` and read the matching `device/hwmon/hwmonN/name == "amdgpu"` `temp1_input`. The card/hwmon layout on this omarchy desktop is `card1` with `hwmon1`; `temp1_input` is in millidegrees and must be divided by 1000.
- Docker Engine API client: the project now issues its own HTTP requests to the Docker daemon via /var/run/docker.sock (or DOCKER_HOST) instead of importing `github.com/docker/docker`. This removes the govulncheck failures flagged against docker/dockers archive-copy/plugin APIs, which we never used. Trade-off: we maintain minimal local structs (`containerSummary`, `containerStats`) matching the Engine API v1.41 response.
- Container stop/restart endpoints in `internal/server` also use a project-owned `DockerController` interface backed by plain HTTP calls to the Docker Engine API (`/containers/{id}/stop` and `/containers/{id}/restart`), mirroring the collector's minimal-client approach. `ErrContainerNotFound` is mapped to HTTP 400 per the task acceptance criteria; other controller errors surface as 500 with a message.
- Docker stats latency dominates the snapshot. The Engine `/containers/{id}/stats?stream=false` call blocks ~1-2 s per container (it samples CPU over one interval), so fetching sequentially froze the whole live page for ~45 s with ~20 containers — the SSE hub sends one frame only after the entire `Assembler.Collect` returns, docker last. `DockerCollector.Collect` now fetches per-container stats concurrently, bounded to `dockerStatsConcurrency = 8`; this cut `Collect` to ~5 s and first paint to ~7 s. Everything else in a full collect is cheap (system ~3 ms, io ~1 ms, 788 processes ~140 ms, per-process disk I/O ~5 ms). The test fake's `statsCalls` slice needed a mutex once fetching went concurrent.
- Confirmation-button CSS is combinator-sensitive. The yes/no buttons live *inside* the confirm `<span>`, which itself carries `.btn`, so a descendant rule like `.confirming .btn { display:none }` also hides the nested yes/no — the confirmation looked broken (text, no buttons). Container confirms compounded it by toggling `confirming-stop`/`confirming-restart` on the row, which the `.confirming …` selectors never matched. Fix: hide only the top-level action buttons via the direct-child combinator (`.confirming > .kill`, `.confirming-stop > .stop`, …) and reveal the matching confirm span the same way. Verified the computed `display` with headless chromium — the JS tests only assert on HTML strings and cannot catch a pure-CSS visibility bug.
