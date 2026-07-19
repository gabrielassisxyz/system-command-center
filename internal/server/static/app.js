(function () {
  'use strict';

  const statusEl = document.getElementById('status');
  const systemEl = document.getElementById('system');
  const processesEl = document.getElementById('processes');
  const dockerEl = document.getElementById('docker');

  function escapeHtml(s) {
    return String(s).replace(/[&<>"']/g, function (ch) {
      switch (ch) {
        case '&': return '&amp;';
        case '<': return '&lt;';
        case '>': return '&gt;';
        case '"': return '&quot;';
        case "'": return '&#39;';
      }
      return ch;
    });
  }

  function fmtPercent(v) {
    if (v === undefined || v === null || isNaN(v)) return '—';
    return v.toFixed(1) + '%';
  }

  function fmtTemp(v) {
    if (v === undefined || v === null || isNaN(v)) return '—';
    return v.toFixed(1) + '°C';
  }

  function fmtBytes(v) {
    if (v === undefined || v === null || isNaN(v)) return '—';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    let i = 0;
    let n = v;
    while (n >= 1024 && i < units.length - 1) {
      n /= 1024;
      i++;
    }
    return n.toFixed(i === 0 ? 0 : 1) + ' ' + units[i];
  }

  function fmtRate(v) {
    if (v === undefined || v === null || isNaN(v)) return '—';
    return fmtBytes(v) + '/s';
  }

  function setStatus(text, state) {
    statusEl.textContent = text;
    statusEl.dataset.state = state;
  }

  // The reference design's card meter is four discrete segments. Each segment
  // covers 25 points of the percentage; a partially covered segment renders at
  // half opacity so the bar reads as an analog level, not a bucket count.
  const METER_SEGMENTS = 4;

  function meter(pct) {
    if (pct === undefined || pct === null || isNaN(pct)) return '';
    const filled = Math.max(0, Math.min(100, pct)) / (100 / METER_SEGMENTS);
    let html = '<div class="meter">';
    for (let i = 0; i < METER_SEGMENTS; i++) {
      const coverage = Math.max(0, Math.min(1, filled - i));
      const cls = coverage >= 0.75 ? ' class="on"' : (coverage >= 0.25 ? ' class="half"' : '');
      html += '<span' + cls + '></span>';
    }
    return html + '</div>';
  }

  function metricCard(label, value, sub, pct) {
    let html = '<div class="card">';
    html += '<h3 class="card-label">' + escapeHtml(label) + '</h3>';
    html += '<div class="card-value">' + escapeHtml(value) + '</div>';
    if (sub) html += '<div class="card-sub">' + escapeHtml(sub) + '</div>';
    html += meter(pct);
    return html + '</div>';
  }

  function ratio(used, total) {
    if (used === undefined || used === null) return null;
    if (!total) return null;
    return (used / total) * 100;
  }

  function renderSystem(sys) {
    const cards = [];

    if (sys.cpu !== undefined && sys.cpu !== null) {
      cards.push(metricCard('CPU', fmtPercent(sys.cpu), '', sys.cpu));
    }

    if (sys.ram && sys.ram.used !== undefined) {
      const sub = sys.ram.total !== undefined ? 'of ' + fmtBytes(sys.ram.total) : '';
      cards.push(metricCard('RAM', fmtBytes(sys.ram.used), sub, ratio(sys.ram.used, sys.ram.total)));
    }

    if (sys.disk_io) {
      // A byte-rate has no ceiling to scale a meter against, so these cards show
      // the raw read/write figures rather than a fabricated percentage.
      cards.push(metricCard('Disk I/O', 'R ' + fmtRate(sys.disk_io.read), 'W ' + fmtRate(sys.disk_io.write), null));
    }

    if (sys.net_io) {
      cards.push(metricCard('Network', 'RX ' + fmtRate(sys.net_io.rx), 'TX ' + fmtRate(sys.net_io.tx), null));
    }

    if (sys.temps && sys.temps.length) {
      // The hottest sensor is the one worth putting on the card; the rest stay
      // in the subtitle count so a missing label never blanks the tile.
      let hottest = null;
      for (let i = 0; i < sys.temps.length; i++) {
        const t = sys.temps[i];
        if (t.value === undefined || t.value === null) continue;
        if (!hottest || t.value > hottest.value) hottest = t;
      }
      if (hottest) {
        cards.push(metricCard('Temp', fmtTemp(hottest.value), hottest.label || 'temp', hottest.value));
      }
    }

    if (sys.gpu) {
      const g = sys.gpu;
      const sub = [];
      if (g.vram_used !== undefined) {
        sub.push('VRAM ' + fmtBytes(g.vram_used) + (g.vram_total !== undefined ? ' / ' + fmtBytes(g.vram_total) : ''));
      }
      if (g.temp !== undefined) sub.push(fmtTemp(g.temp));
      if (g.busy !== undefined || sub.length) {
        cards.push(metricCard('GPU', g.busy !== undefined ? 'busy ' + fmtPercent(g.busy) : '—', sub.join(' · '), g.busy));
      }
    }

    systemEl.innerHTML = cards.join('');
  }

  function sortProcesses(rows, key) {
    return rows.slice().sort(function (a, b) {
      let va = 0;
      let vb = 0;
      switch (key) {
        case 'ram':
          va = (a.ram && a.ram.used) || 0;
          vb = (b.ram && b.ram.used) || 0;
          break;
        case 'cpu':
          va = a.cpu || 0;
          vb = b.cpu || 0;
          break;
        case 'disk_io':
          va = ((a.disk_io && a.disk_io.read) || 0) + ((a.disk_io && a.disk_io.write) || 0);
          vb = ((b.disk_io && b.disk_io.read) || 0) + ((b.disk_io && b.disk_io.write) || 0);
          break;
        default:
          va = (a.ram && a.ram.used) || 0;
          vb = (b.ram && b.ram.used) || 0;
      }
      return vb - va;
    });
  }

  function sortByRamDesc(rows) {
    return sortProcesses(rows, 'ram');
  }

  function panel(title, control, body) {
    return '<div class="panel">' +
      '<div class="panel-head"><h2>' + escapeHtml(title) + '</h2>' + (control || '') + '</div>' +
      body +
      '</div>';
  }

  // Which process groups the user has expanded, keyed by program name. Groups
  // default to collapsed — the whole point is one line per program — and the
  // state lives outside the DOM so a re-render every 2s does not fold it back.
  const expandedProcGroups = {};

  // aggregateProcesses folds the flat process list into one entry per program
  // name, summing CPU/RAM/disk. A program that runs once (e.g. systemd) stays a
  // single-PID group and renders as a plain row.
  function aggregateProcesses(rows) {
    const byName = {};
    for (let i = 0; i < rows.length; i++) {
      const p = rows[i];
      let g = byName[p.name];
      if (!g) {
        g = byName[p.name] = { name: p.name, procs: [], cpu: 0, ram: 0, diskRead: 0, diskWrite: 0 };
      }
      g.procs.push(p);
      g.cpu += p.cpu || 0;
      g.ram += (p.ram && p.ram.used) || 0;
      g.diskRead += (p.disk_io && p.disk_io.read) || 0;
      g.diskWrite += (p.disk_io && p.disk_io.write) || 0;
    }
    return Object.keys(byName).map(function (k) { return byName[k]; });
  }

  function groupSortValue(g, key) {
    switch (key) {
      case 'cpu': return g.cpu;
      case 'disk_io': return g.diskRead + g.diskWrite;
      default: return g.ram;
    }
  }

  function sortProcessGroups(groups, key) {
    return groups.slice().sort(function (a, b) {
      return groupSortValue(b, key) - groupSortValue(a, key);
    });
  }

  // The RAM bar is relative to the heaviest program this frame — an absolute
  // scale would leave every bar invisible on a 64 GB machine.
  function ramCell(used, maxRam) {
    const label = (used !== null && used !== undefined) ? fmtBytes(used) : '—';
    const width = (used && maxRam) ? Math.round((used / maxRam) * 100) : 0;
    return '<div class="ram-cell"><span class="bar"><i style="width:' + width +
      '%"></i></span>' + label + '</div>';
  }

  function diskCell(read, write) {
    return 'R ' + fmtRate(read) + '<br>W ' + fmtRate(write);
  }

  // The detail is what identifies one of 27 "brave" or 12 "node" rows: the
  // Chromium role, the script, the PostgreSQL backend title, the working
  // directory. Absent for processes that have nothing to add.
  function detailSpan(detail) {
    if (!detail) return '';
    return '<span class="proc-detail">' + escapeHtml(detail) + '</span>';
  }

  function procRow(p, maxRam) {
    const used = (p.ram && p.ram.used !== undefined) ? p.ram.used : null;
    return '<tr data-pid="' + p.pid + '">' +
      '<td><div class="proc-name">' + escapeHtml(p.name) + detailSpan(p.detail) + '</div></td>' +
      '<td>' + p.pid + '</td>' +
      '<td class="num accent">' + fmtPercent(p.cpu) + '</td>' +
      '<td class="num">' + ramCell(used, maxRam) + '</td>' +
      '<td class="num">' + diskCell(p.disk_io && p.disk_io.read, p.disk_io && p.disk_io.write) + '</td>' +
      '<td>' + killButton(p.pid) + '</td>' +
      '</tr>';
  }

  // The child rows are linked to their header by numeric index, not by program
  // name: a name can hold spaces or quotes that would break an attribute
  // selector, whereas an index is always selector-safe.
  function procGroupRow(g, idx, expanded, maxRam) {
    return '<tr class="proc-group-row" data-proc-idx="' + idx + '"' +
      ' data-proc-name="' + escapeHtml(g.name) + '"' +
      ' aria-expanded="' + (expanded ? 'true' : 'false') + '">' +
      '<td><div class="proc-group-name"><span class="caret">▾</span>' +
      escapeHtml(g.name) + '<span class="proc-count">×' + g.procs.length + '</span></div></td>' +
      '<td></td>' +
      '<td class="num accent">' + fmtPercent(g.cpu) + '</td>' +
      '<td class="num">' + ramCell(g.ram, maxRam) + '</td>' +
      '<td class="num">' + diskCell(g.diskRead, g.diskWrite) + '</td>' +
      '<td></td>' +
      '</tr>';
  }

  function procChildRow(p, idx, expanded, maxRam) {
    const used = (p.ram && p.ram.used !== undefined) ? p.ram.used : null;
    return '<tr class="proc-child-row" data-proc-parent="' + idx + '" data-pid="' + p.pid + '"' +
      (expanded ? '' : ' hidden') + '>' +
      '<td><div class="proc-child-name">' + escapeHtml(p.name) + detailSpan(p.detail) + '</div></td>' +
      '<td>' + p.pid + '</td>' +
      '<td class="num accent">' + fmtPercent(p.cpu) + '</td>' +
      '<td class="num">' + ramCell(used, maxRam) + '</td>' +
      '<td class="num">' + diskCell(p.disk_io && p.disk_io.read, p.disk_io && p.disk_io.write) + '</td>' +
      '<td>' + killButton(p.pid) + '</td>' +
      '</tr>';
  }

  function renderProcesses(processes, sortKey) {
    const key = sortKey || 'ram';
    const control = document.getElementById('process-sort-control').innerHTML;
    const groups = sortProcessGroups(aggregateProcesses(processes), key);

    if (!groups.length) {
      processesEl.innerHTML = panel('Programs & Processes', control, '<p class="panel-empty">No processes.</p>');
      return;
    }

    // Scale the bars against the heaviest aggregated group; child rows share the
    // same scale so a PID's bar reads as its slice of the machine, not its group.
    let maxRam = 0;
    for (let i = 0; i < groups.length; i++) {
      if (groups[i].ram > maxRam) maxRam = groups[i].ram;
    }

    let rows = '<div class="panel-body"><table><thead><tr>';
    rows += '<th>Process</th><th>PID</th><th class="num">CPU</th><th class="num">RAM</th><th class="num">Disk I/O</th><th>Actions</th>';
    rows += '</tr></thead><tbody>';

    for (let i = 0; i < groups.length; i++) {
      const g = groups[i];
      if (g.procs.length === 1) {
        rows += procRow(g.procs[0], maxRam);
        continue;
      }
      const expanded = !!expandedProcGroups[g.name];
      rows += procGroupRow(g, i, expanded, maxRam);
      const children = sortProcesses(g.procs, key);
      for (let j = 0; j < children.length; j++) {
        rows += procChildRow(children[j], i, expanded, maxRam);
      }
    }

    rows += '</tbody></table></div>';
    processesEl.innerHTML = panel('Programs & Processes', control, rows);

    const select = processesEl.querySelector('[data-sort-select]');
    if (select) {
      select.value = key;
      select.addEventListener('change', function () {
        // Persist the choice: the next SSE frame re-renders from currentSortKey,
        // and without this the list would snap back to RAM within a second.
        currentSortKey = select.value;
        renderProcesses(processes, currentSortKey);
      });
    }
    bindProcGroupRows(processesEl);
    bindKillButtons(processesEl);
  }

  function bindProcGroupRows(root) {
    root.querySelectorAll('[data-proc-idx]').forEach(function (row) {
      row.addEventListener('click', function () {
        const idx = row.dataset.procIdx;
        const name = row.dataset.procName;
        const expanded = !expandedProcGroups[name];
        expandedProcGroups[name] = expanded;
        row.setAttribute('aria-expanded', expanded ? 'true' : 'false');
        root.querySelectorAll('[data-proc-parent="' + idx + '"]').forEach(function (child) {
          child.hidden = !expanded;
        });
      });
    });
  }

  function killButton(pid) {
    return '<div class="btn-row" data-kill-row="' + pid + '">' +
      '<button class="btn kill" data-kill-pid="' + pid + '" title="Send SIGTERM to process ' + pid + '">kill</button>' +
      '<span class="btn confirm">kill ' + pid + '? ' +
      '<button class="btn yes" data-kill-yes="' + pid + '">yes</button>' +
      '<button class="btn no" data-kill-no="' + pid + '">no</button>' +
      '</span></div>';
  }

  function bindKillButtons(root) {
    root.querySelectorAll('[data-kill-pid]').forEach(function (btn) {
      btn.addEventListener('click', function () {
        const row = root.querySelector('[data-kill-row="' + btn.dataset.killPid + '"]');
        if (row) row.classList.add('confirming');
      });
    });
    root.querySelectorAll('[data-kill-yes]').forEach(function (btn) {
      btn.addEventListener('click', function () {
        postKill(parseInt(btn.dataset.killYes, 10));
        const row = root.querySelector('[data-kill-row="' + btn.dataset.killYes + '"]');
        if (row) row.classList.remove('confirming');
      });
    });
    root.querySelectorAll('[data-kill-no]').forEach(function (btn) {
      btn.addEventListener('click', function () {
        const row = root.querySelector('[data-kill-row="' + btn.dataset.killNo + '"]');
        if (row) row.classList.remove('confirming');
      });
    });
  }

  function postKill(pid) {
    fetch('/api/process/kill?pid=' + pid, { method: 'POST' }).then(function (res) {
      if (!res.ok) {
        console.error('kill request failed:', res.status);
        alert('Failed to kill process ' + pid + ': ' + res.status);
      }
    }).catch(function (err) {
      console.error('kill request error:', err);
      alert('Failed to kill process ' + pid + ': ' + err);
    });
  }

  // Stop/restart are reversible, so they fire on the first click with no
  // confirmation. The optimistic "stopping…" label below is the feedback that
  // the request is in flight.
  function containerButtons(id) {
    const short = escapeHtml(shortID(id));
    return '<div class="btn-row" data-container-row="' + escapeHtml(id) + '">' +
      '<button class="btn stop" data-container-stop="' + escapeHtml(id) + '" title="Stop container ' + short + '">stop</button>' +
      '<button class="btn restart" data-container-restart="' + escapeHtml(id) + '" title="Restart container ' + short + '">restart</button>' +
      '</div>';
  }

  // In-flight stop/restart requests, keyed by container id, each with an expiry.
  // A frame rebuilds the table from scratch, so this state (like the fold state)
  // has to live outside the DOM or the label would vanish on the next SSE frame.
  const pendingContainers = {};
  const PENDING_TTL_MS = 10000;

  function statusLabel(action) {
    return '<span class="btn-status">' + (action === 'stop' ? 'stopping…' : 'restarting…') + '</span>';
  }

  // containerActions is what the table renders: the pending label while a
  // request is outstanding, otherwise the action buttons.
  function containerActions(id) {
    const p = pendingContainers[id];
    if (p && p.until > Date.now()) return statusLabel(p.action);
    return containerButtons(id);
  }

  function prunePending() {
    const now = Date.now();
    for (const id in pendingContainers) {
      if (pendingContainers[id].until <= now) delete pendingContainers[id];
    }
  }

  function bindContainerButtons(root) {
    root.querySelectorAll('[data-container-stop]').forEach(function (btn) {
      btn.addEventListener('click', function () { runContainerAction('stop', btn.dataset.containerStop); });
    });
    root.querySelectorAll('[data-container-restart]').forEach(function (btn) {
      btn.addEventListener('click', function () { runContainerAction('restart', btn.dataset.containerRestart); });
    });
  }

  // runContainerAction records the pending state, paints the label immediately
  // (so feedback does not wait for the next SSE frame), then posts the request.
  function runContainerAction(action, id) {
    pendingContainers[id] = { action: action, until: Date.now() + PENDING_TTL_MS };
    const row = dockerEl.querySelector('[data-container-row="' + id + '"]');
    if (row) row.outerHTML = statusLabel(action);
    postContainerAction(action, id);
  }

  function postContainerAction(action, id) {
    fetch('/api/container/' + action + '?id=' + encodeURIComponent(id), { method: 'POST' }).then(function (res) {
      if (!res.ok) {
        delete pendingContainers[id];
        console.error('container ' + action + ' failed:', res.status);
        alert('Failed to ' + action + ' container ' + id + ': ' + res.status);
      }
    }).catch(function (err) {
      delete pendingContainers[id];
      console.error('container ' + action + ' error:', err);
      alert('Failed to ' + action + ' container ' + id + ': ' + err);
    });
  }

  // Which stacks the user has collapsed, keyed by compose project. Every SSE
  // frame rebuilds the table from scratch, so the fold state has to live
  // outside the DOM or it would spring back open once a second.
  const collapsedStacks = {};

  // A stack row shows one pulsing dot per container, capped so a 30-container
  // stack does not push the CPU and RAM columns off screen.
  const MAX_STACK_DOTS = 6;

  // Docker's own short-id length. The full 64-char id stays in the data
  // attributes the action endpoints read; displaying it would push the CPU and
  // RAM columns off the right edge of the table.
  const SHORT_ID_LEN = 12;

  function shortID(id) {
    return String(id).slice(0, SHORT_ID_LEN);
  }

  function stackDots(count) {
    let html = '<span class="dots">';
    for (let i = 0; i < Math.min(count, MAX_STACK_DOTS); i++) html += '<i></i>';
    html += '</span>';
    if (count > MAX_STACK_DOTS) html += '<span class="container-id">+' + (count - MAX_STACK_DOTS) + '</span>';
    return html;
  }

  function renderDocker(groups) {
    prunePending();

    if (!groups || !groups.length) {
      dockerEl.innerHTML = panel('Docker Stacks', '', '<p class="panel-empty">No Docker containers.</p>');
      return;
    }

    let rows = '<div class="panel-body"><table><thead><tr>';
    rows += '<th>Stack / Container</th><th>ID</th><th class="num">CPU</th><th class="num">RAM</th><th>Actions</th>';
    rows += '</tr></thead><tbody>';

    for (let i = 0; i < groups.length; i++) {
      const g = groups[i];
      const collapsed = !!collapsedStacks[g.project];

      let cpuTotal = 0;
      let ramTotal = 0;
      for (let j = 0; j < g.containers.length; j++) {
        cpuTotal += g.containers[j].cpu || 0;
        ramTotal += (g.containers[j].ram && g.containers[j].ram.used) || 0;
      }

      // Container ids are 64-char hex, so a comma join is a safe, selector-free
      // way to hand the whole stack to the stop-all handler.
      const ids = g.containers.map(function (c) { return c.id; }).join(',');

      rows += '<tr class="stack-row" data-stack="' + escapeHtml(g.project) + '"' +
        ' aria-expanded="' + (collapsed ? 'false' : 'true') + '">';
      rows += '<td><div class="stack-name"><span class="caret">▾</span>' +
        escapeHtml(g.project) + stackDots(g.containers.length) + '</div></td>';
      rows += '<td></td>';
      rows += '<td class="num accent">' + fmtPercent(cpuTotal) + '</td>';
      rows += '<td class="num">' + fmtBytes(ramTotal) + '</td>';
      rows += '<td><button class="btn stop" data-stack-stop="' + escapeHtml(ids) + '"' +
        ' title="Stop all ' + g.containers.length + ' containers in ' + escapeHtml(g.project) + '">stop all</button></td>';
      rows += '</tr>';

      for (let j = 0; j < g.containers.length; j++) {
        const c = g.containers[j];
        const ram = (c.ram && c.ram.used !== undefined) ? fmtBytes(c.ram.used) : '—';
        rows += '<tr class="container-row" data-container-id="' + escapeHtml(c.id) + '"' +
          ' data-stack-child="' + escapeHtml(g.project) + '"' + (collapsed ? ' hidden' : '') + '>';
        rows += '<td><div class="container-name">' + escapeHtml(c.name) + '</div></td>';
        rows += '<td class="container-id">' + escapeHtml(shortID(c.id)) + '</td>';
        rows += '<td class="num accent">' + fmtPercent(c.cpu) + '</td>';
        rows += '<td class="num">' + ram + '</td>';
        rows += '<td>' + containerActions(c.id) + '</td>';
        rows += '</tr>';
      }
    }

    rows += '</tbody></table></div>';
    dockerEl.innerHTML = panel('Docker Stacks', '', rows);

    bindStackRows(dockerEl);
    bindStackStop(dockerEl);
    bindContainerButtons(dockerEl);
  }

  function bindStackStop(root) {
    root.querySelectorAll('[data-stack-stop]').forEach(function (btn) {
      btn.addEventListener('click', function (e) {
        // Stop the propagation so stopping the stack does not also toggle its fold.
        e.stopPropagation();
        const ids = btn.dataset.stackStop ? btn.dataset.stackStop.split(',') : [];
        ids.forEach(function (id) { runContainerAction('stop', id); });
      });
    });
  }

  function bindStackRows(root) {
    root.querySelectorAll('[data-stack]').forEach(function (row) {
      row.addEventListener('click', function () {
        const project = row.dataset.stack;
        const collapsed = !collapsedStacks[project];
        collapsedStacks[project] = collapsed;
        row.setAttribute('aria-expanded', collapsed ? 'false' : 'true');
        root.querySelectorAll('[data-stack-child="' + project + '"]').forEach(function (child) {
          child.hidden = collapsed;
        });
      });
    });
  }

  let currentSortKey = 'ram';

  function render(snapshot) {
    renderSystem(snapshot.system || {});
    renderDocker(snapshot.docker || []);
    renderProcesses(snapshot.processes || [], currentSortKey);
  }

  function connect() {
    setStatus('Connecting…', 'connecting');
    const es = new EventSource('/events');

    es.addEventListener('open', function () {
      setStatus('Live', 'live');
    });

    es.addEventListener('message', function (ev) {
      try {
        render(JSON.parse(ev.data));
      } catch (err) {
        console.error('failed to parse snapshot:', err);
      }
    });

    es.addEventListener('error', function () {
      setStatus('Reconnecting…', 'lost');
    });
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', connect);
  } else {
    connect();
  }
})();
