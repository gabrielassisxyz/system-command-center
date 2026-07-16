# task_plan.md — hardware-usage

> Executable checklist. One atomic task per line. The loop implements the first
> `[ ]` it finds, top to bottom. Every task carries a `done when:` acceptance check.
> Read `findings.md` first — it holds the load-bearing decisions this plan assumes.

## Phase 1 — Server skeleton & data model

- [x] Snapshot data model — Go structs in `internal/model` for the full snapshot (system
  header, per-process rows, docker groups) with JSON tags; absent metrics representable
  (pointers / `omitempty`). — done when: `go test ./internal/model` marshals a
  fully-populated Snapshot and a metrics-absent Snapshot to JSON, asserting expected keys
  and that absent fields are null/omitted.
- [x] Loopback HTTP server + embedded static page — `net/http` server bound to
  `127.0.0.1:8765`, serving an embedded `index.html` (via `embed`). — done when: an
  httptest GET `/` returns 200 with the HTML body, a test asserts the listen address is
  `127.0.0.1:8765`, and `go vet ./...` is clean.
- [x] SSE hub — `/events` streams `data: <json>\n\n` frames; a hub tracks subscribers and
  runs a ~2s ticker that broadcasts the latest snapshot **only while ≥1 subscriber is
  connected** (snapshot supplied by an injected provider; stub provider for now). — done
  when: a test connects two httptest clients and both receive a frame within one tick, and
  asserts the provider is NOT called once all subscribers disconnect.

## Phase 2 — Metric collectors (each behind a thin interface, fake-tested)

- [x] System metrics collector — CPU %, RAM used/total, temperatures via `gopsutil`,
  behind a `SystemSource` interface this project owns. — done when: `go test` with a fake
  source yields a system header with the expected values, and missing temperatures come
  back absent (not an error).
- [x] System I/O rate collector — disk + network cumulative counters diffed across two
  `Collect()` calls into read/write and rx/tx rates, using an injectable clock. — done
  when: a test calls Collect twice with fake counters + a controlled clock and asserts
  rate == delta/deltaT, and that the first call (no previous sample) yields zero rates.
- [x] Per-process collector — one row per process (pid, name, CPU %, RAM/RSS) via
  `gopsutil`, behind a `ProcessSource` interface. — done when: a test with a fake source
  returns rows with the expected fields, and a process whose metric read errors is
  skipped/marked absent rather than aborting the whole list.
- [x] Per-process disk I/O — read/write rate per pid from `/proc/<pid>/io`, diffed across
  calls; permission-denied on a pid → that pid's disk I/O absent, not fatal. — done when:
  a test with a fake io-source (some pids denied) yields diffed rates for readable pids and
  absent for denied ones, with no error surfaced.
- [x] AMD GPU collector — read `gpu_busy_percent`, `mem_info_vram_used/total`, and hwmon
  temperature from an injectable sysfs root; every field absent-tolerant. — done when: a
  test pointing the reader at a fixture dir returns busy%/VRAM/temp, and a dir missing
  those files returns GPU fields absent with no crash.
- [x] Docker collector — list containers with per-container CPU % + RAM from the docker
  client, grouped by the `com.docker.compose.project` label; unlabeled → "(ungrouped)";
  behind a `DockerSource` interface. — done when: a test with a fake source returns groups
  keyed by project with their containers, unlabeled containers under "(ungrouped)", and
  docker-unavailable yields empty groups (no crash).

## Phase 3 — Assembly & actions

- [x] Snapshot assembler — combine the system, per-process, GPU, and docker collectors into
  one `Snapshot`; stateful, holding the previous sample so rates are computed on assembly.
  — done when: a test with all-fake sources produces a complete Snapshot, and a second call
  yields non-zero rates where counters advanced.
- [x] Wire assembler into the SSE hub — replace the stub provider with the real assembler so
  collection runs on the hub tick only while clients are connected. — done when: an httptest
  client on `/events` receives a real Snapshot JSON frame (fakes injected) with header,
  processes, and docker groups all populated.
- [ ] Kill action endpoint — `POST /api/process/kill?pid=<n>` sends SIGTERM via an
  injectable killer. — done when: an httptest POST invokes the killer with the parsed pid; a
  missing/bad pid returns 400; a killer error surfaces as a 5xx with a message, not a panic.
- [ ] Docker stop/restart endpoints — `POST /api/container/stop` and
  `POST /api/container/restart` by container id via an injectable docker controller. — done
  when: httptest POSTs invoke stop/restart with the id; a missing/unknown id returns 400; a
  controller error surfaces as an error status.

## Phase 4 — Frontend & done-check

- [ ] Frontend render — `index.html` + `app.js` subscribe to `/events` and render the system
  header, the per-process list, and the Docker groups, each row showing its metric. — done
  when: loading the page against a running `/events` shows the header + both lists updating
  live (observed via `bin/dev`).
- [ ] Sort control — heaviest-first, RAM by default, with a control toggling the per-row sort
  metric among RAM / CPU / disk I/O. — done when: the process list loads RAM-descending by
  default and re-sorts when the control changes (observed via `bin/dev`).
- [ ] Action buttons — a kill button per process and stop/restart buttons per container,
  each calling its POST endpoint behind a confirmation. — done when: clicking kill POSTs the
  pid, clicking stop/restart POSTs the container id, and the row reflects the change on the
  next snapshot (observed via `bin/dev`).
- [ ] `bin/dev` + end-to-end done-check — `bin/dev` runs the server; `bin/dev`/README note
  the `sudo` requirement for complete per-process disk I/O. — done when: `bin/dev` starts the
  server on `127.0.0.1:8765`, opening it shows a live system header + per-program list +
  Docker-grouped list sorted by RAM by default (the IDEA done-check passes), and `bin/ci` is
  green.
