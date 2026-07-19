# IDEA — hardware-usage

## WHAT (one line)
A web page that lets me watch my omarchy desktop's resource usage in real time, listed per program and with Docker containers grouped by their compose stack, so I can see at a glance what is eating my RAM (and every other metric).

## Anti-goal (one line)
It is NOT a monitoring/observability platform (no Grafana/Prometheus), NOT multi-user, NOT a critical 24/7 daemon — and v1 keeps NO history: no background collector, no storage, no time-series charts (history is deferred to v2).

## Done-check (one concrete verification)
Open the site and see the live hardware metrics of this desktop, sorted by highest usage of a chosen metric (RAM by default) — both the per-program list and the Docker containers grouped by compose stack, each showing its own usage.

## Constraints
- **Scope:** only this machine — the omarchy desktop (Arch/Hyprland). No homelab, no remote hosts.
- **Interface:** a web page (browser), not a TUI or CSV.
- **Live only:** data is read on demand while the page is open and pushed to the browser; nothing is persisted.
- **Metrics:** all of them (CPU, RAM, disk, temperature, etc.), but RAM is the motivating/default sort key.
- **Grouping is the point:** per-program process listing AND Docker containers grouped by compose stack, drillable to the individual container.

## Notes / reframe
- **Reframe (accepted):** this is not "htop in a browser" — `btop` already does live process monitoring better than we would. The unique value is the **topology-aware live view**: usage grouped the way I think about my machine — per program, and Docker containers grouped by their compose stack. Everything that doesn't serve that view gets cut.
- **History decision:** Gabriel initially wanted history too ("ambos"), imagined as something lightweight. But any history needs a 24/7 collector + storage, which directly contradicts the confirmed anti-goal. Decision: **v1 is live-only**; history is a v2 idea, to be built only if the need proves real.
- **Data sources (implementation hint, not a commitment):** process/system metrics from `/proc` (or a library over it); container metrics from the Docker API / `docker stats`; compose-stack grouping via container labels (`com.docker.compose.project`).
