# hardware-usage

A live, local-only web page for watching this desktop's resource usage — a per-process
list and Docker containers grouped by their compose stack, sorted by whatever metric is
eating the machine (RAM by default). No history, no storage: data is read on demand while
the page is open and pushed to the browser over SSE.

See `IDEA.md` for the full intent and `AGENTS.md` for how the repo is built.

## Run

```sh
bin/install-hooks   # once, after clone — installs the gitleaks pre-commit hook
bin/dev             # serves http://127.0.0.1:8765
```

Open http://127.0.0.1:8765: a live system header (CPU, RAM, disk & network I/O,
temperature, AMD GPU), a per-process list sorted heaviest-first (RAM by default,
toggle to CPU or disk I/O), and Docker containers grouped by their compose stack.
Each process has a kill button (SIGTERM); each container has stop/restart — both
behind a confirmation.

Per-process disk I/O reads `/proc/<pid>/io`, which the kernel restricts to the
process owner. Run `bin/dev` unprivileged and you see disk I/O for your own
processes only; run `sudo bin/dev` for complete per-process disk I/O across the
machine. Everything else (system header, container stats, kill/stop/restart) works
without `sudo`.

## Develop

`bin/ci` is the single gate (gofmt → vet → golangci-lint → test → govulncheck); CI runs
the same script on `main`.
