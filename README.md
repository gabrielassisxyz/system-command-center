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

## Develop

`bin/ci` is the single gate (gofmt → vet → golangci-lint → test → govulncheck); CI runs
the same script on `main`.
