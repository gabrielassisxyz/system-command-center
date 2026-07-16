(function () {
  'use strict';

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

  function renderSystem(sys) {
    const parts = [];
    parts.push('<h2>System</h2>');
    parts.push('<table>');
    parts.push('<tr><th>CPU</th><td class="num">' + fmtPercent(sys.cpu) + '</td></tr>');

    if (sys.ram) {
      let ram = '—';
      if (sys.ram.used !== undefined && sys.ram.total !== undefined) {
        ram = fmtBytes(sys.ram.used) + ' / ' + fmtBytes(sys.ram.total);
      } else if (sys.ram.used !== undefined) {
        ram = fmtBytes(sys.ram.used);
      }
      parts.push('<tr><th>RAM</th><td class="num">' + ram + '</td></tr>');
    }

    if (sys.disk_io) {
      const r = fmtRate(sys.disk_io.read);
      const w = fmtRate(sys.disk_io.write);
      parts.push('<tr><th>Disk I/O</th><td class="num">R ' + r + ' / W ' + w + '</td></tr>');
    }

    if (sys.net_io) {
      const rx = fmtRate(sys.net_io.rx);
      const tx = fmtRate(sys.net_io.tx);
      parts.push('<tr><th>Network</th><td class="num">RX ' + rx + ' / TX ' + tx + '</td></tr>');
    }

    if (sys.temps && sys.temps.length) {
      const temps = sys.temps
        .map(function (t) { return escapeHtml(t.label || 'temp') + ': ' + fmtTemp(t.value); })
        .join(', ');
      parts.push('<tr><th>Temps</th><td>' + temps + '</td></tr>');
    }

    if (sys.gpu) {
      const g = sys.gpu;
      const bits = [];
      if (g.busy !== undefined) bits.push('busy ' + fmtPercent(g.busy));
      if (g.vram_used !== undefined && g.vram_total !== undefined) {
        bits.push('VRAM ' + fmtBytes(g.vram_used) + ' / ' + fmtBytes(g.vram_total));
      } else if (g.vram_used !== undefined) {
        bits.push('VRAM ' + fmtBytes(g.vram_used));
      }
      if (g.temp !== undefined) bits.push(fmtTemp(g.temp));
      if (bits.length) {
        parts.push('<tr><th>GPU</th><td>' + bits.join(', ') + '</td></tr>');
      }
    }

    parts.push('</table>');
    systemEl.innerHTML = parts.join('');
  }

  function sortByRamDesc(rows) {
    return rows.slice().sort(function (a, b) {
      const ra = (a.ram && a.ram.used) || 0;
      const rb = (b.ram && b.ram.used) || 0;
      return rb - ra;
    });
  }

  function renderProcesses(processes) {
    const sorted = sortByRamDesc(processes);

    let html = '<h2>Processes</h2>';
    if (!sorted.length) {
      processesEl.innerHTML = html + '<p>No processes.</p>';
      return;
    }

    html += '<table><thead><tr>';
    html += '<th>Name</th><th>PID</th><th class="num">CPU</th><th class="num">RAM</th><th class="num">Disk I/O</th>';
    html += '</tr></thead><tbody>';

    for (let i = 0; i < sorted.length; i++) {
      const p = sorted[i];
      const ram = (p.ram && p.ram.used !== undefined) ? fmtBytes(p.ram.used) : '—';
      const diskRead = fmtRate(p.disk_io && p.disk_io.read);
      const diskWrite = fmtRate(p.disk_io && p.disk_io.write);
      html += '<tr data-pid="' + p.pid + '">';
      html += '<td>' + escapeHtml(p.name) + '</td>';
      html += '<td>' + p.pid + '</td>';
      html += '<td class="num">' + fmtPercent(p.cpu) + '</td>';
      html += '<td class="num">' + ram + '</td>';
      html += '<td class="num">R ' + diskRead + '<br>W ' + diskWrite + '</td>';
      html += '</tr>';
    }

    html += '</tbody></table>';
    processesEl.innerHTML = html;
  }

  function renderDocker(groups) {
    let html = '<h2>Docker</h2>';
    if (!groups || !groups.length) {
      dockerEl.innerHTML = html + '<p>No Docker containers.</p>';
      return;
    }

    for (let i = 0; i < groups.length; i++) {
      const g = groups[i];
      html += '<h3>' + escapeHtml(g.project) + '</h3>';
      html += '<table><thead><tr>';
      html += '<th>Name</th><th>ID</th><th class="num">CPU</th><th class="num">RAM</th>';
      html += '</tr></thead><tbody>';

      for (let j = 0; j < g.containers.length; j++) {
        const c = g.containers[j];
        const ram = (c.ram && c.ram.used !== undefined) ? fmtBytes(c.ram.used) : '—';
        html += '<tr data-container-id="' + escapeHtml(c.id) + '">';
        html += '<td>' + escapeHtml(c.name) + '</td>';
        html += '<td>' + escapeHtml(c.id) + '</td>';
        html += '<td class="num">' + fmtPercent(c.cpu) + '</td>';
        html += '<td class="num">' + ram + '</td>';
        html += '</tr>';
      }

      html += '</tbody></table>';
    }

    dockerEl.innerHTML = html;
  }

  function render(snapshot) {
    renderSystem(snapshot.system || {});
    renderProcesses(snapshot.processes || []);
    renderDocker(snapshot.docker || []);
  }

  function connect() {
    systemEl.textContent = 'Connecting…';
    const es = new EventSource('/events');

    es.addEventListener('open', function () {
      systemEl.textContent = 'Connected';
    });

    es.addEventListener('message', function (ev) {
      try {
        render(JSON.parse(ev.data));
      } catch (err) {
        console.error('failed to parse snapshot:', err);
      }
    });

    es.addEventListener('error', function () {
      systemEl.textContent = 'Connection lost. Retrying…';
    });
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', connect);
  } else {
    connect();
  }
})();
