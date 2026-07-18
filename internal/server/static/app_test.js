/*
Static-asset rendering tests for app.js.
Run with Node.js: node internal/server/static/app_test.js
The tests exercise format helpers and render() by snapshotting HTML output
so the frontend contract is covered without a browser.
*/

const fs = require('fs');
const path = require('path');

const appPath = path.join(__dirname, 'app.js');
const appSource = fs.readFileSync(appPath, 'utf8');

// Create a minimal DOM-like environment that app.js expects.
const dom = {
  status: { textContent: '', dataset: {} },
  system: { innerHTML: '' },
  processes: { innerHTML: '', querySelector: function () { return null; }, querySelectorAll: function () { return []; } },
  docker: { innerHTML: '', querySelectorAll: function () { return []; } },
};

global.document = {
  readyState: 'complete',
  getElementById: function (id) {
    if (id === 'process-sort-control') {
      return {
        innerHTML: '<label class="sort-control">Sort by <select data-sort-select><option value="ram">RAM</option><option value="cpu">CPU</option><option value="disk_io">Disk I/O</option></select></label>',
      };
    }
    return dom[id];
  },
  addEventListener: function () {},
};

global.EventSource = function () {
  this.listeners = {};
};
global.EventSource.prototype.addEventListener = function (name, fn) {
  this.listeners[name] = fn;
};

global.console = { log: console.log, error: console.error };

// Load app.js. The IIFE executes on load and calls connect(), but no events fire.
eval(appSource); // eslint-disable-line no-eval

// The IIFE above already populated the local scope of the eval with helpers,
// but that scope is inaccessible. Re-run the app source as a function body
// that returns the helpers we need. We must replace the initial IIFE wrapper
// so the declarations leak into the returned object.
const body = appSource
  .replace(/^\(function \(\) \{/, '')
  .replace(/\}\)\(\);\s*$/, '');
const helperSource =
  '(function () {\n' +
  body +
  '\nreturn { escapeHtml: escapeHtml, fmtPercent: fmtPercent, fmtTemp: fmtTemp, fmtBytes: fmtBytes, fmtRate: fmtRate, sortProcesses: sortProcesses, sortByRamDesc: sortByRamDesc, renderSystem: renderSystem, renderProcesses: renderProcesses, renderDocker: renderDocker, render: render, killButton: killButton, containerButtons: containerButtons, meter: meter };' +
  '\n})()';
const helpers = eval(helperSource); // eslint-disable-line no-eval

let failures = 0;

function assertEq(label, got, want) {
  if (got !== want) {
    failures++;
    console.error('FAIL ' + label + ':\n  got:  ' + got + '\n  want: ' + want);
  }
}

function assertContains(label, got, want) {
  if (!String(got).includes(want)) {
    failures++;
    console.error('FAIL ' + label + ':\n  got:  ' + got + '\n  want contains: ' + want);
  }
}

assertEq('fmtPercent happy', helpers.fmtPercent(12.345), '12.3%');
assertEq('fmtPercent absent', helpers.fmtPercent(null), '—');
assertEq('fmtTemp happy', helpers.fmtTemp(42.5), '42.5°C');
assertEq('fmtBytes B', helpers.fmtBytes(100), '100 B');
assertEq('fmtBytes KB', helpers.fmtBytes(1536), '1.5 KB');
assertEq('fmtBytes MB', helpers.fmtBytes(2 * 1024 * 1024), '2.0 MB');
assertEq('fmtBytes GB', helpers.fmtBytes(3 * 1024 * 1024 * 1024), '3.0 GB');
assertEq('fmtRate', helpers.fmtRate(1024), '1.0 KB/s');
assertEq('escapeHtml', helpers.escapeHtml('<b>"x"\'s</b>'), '&lt;b&gt;&quot;x&quot;&#39;s&lt;/b&gt;');

const sorted = helpers.sortByRamDesc([
  { pid: 1, ram: { used: 100 } },
  { pid: 2, ram: { used: 500 } },
  { pid: 3 },
]);
assertEq('sortByRamDesc first', sorted[0].pid, 2);
assertEq('sortByRamDesc last', sorted[2].pid, 3);

helpers.render({
  system: {
    cpu: 25.5,
    ram: { used: 8 * 1024 * 1024 * 1024, total: 16 * 1024 * 1024 * 1024 },
    disk_io: { read: 1024, write: 2048 },
    net_io: { rx: 512, tx: 256 },
    temps: [{ label: 'Tctl', value: 55.0 }],
    gpu: { busy: 80, vram_used: 4 * 1024 * 1024 * 1024, vram_total: 8 * 1024 * 1024 * 1024, temp: 60.0 },
  },
  processes: [
    { pid: 1, name: 'init', cpu: 1.5, ram: { used: 1 * 1024 * 1024 }, disk_io: { read: 100, write: 200 } },
    { pid: 42, name: 'browser', cpu: 30, ram: { used: 512 * 1024 * 1024 } },
  ],
  docker: [
    { project: 'core', containers: [{ id: 'c1', name: 'app', cpu: 5, ram: { used: 128 * 1024 * 1024 } }] },
    { project: '(ungrouped)', containers: [{ id: 'c2', name: 'lonely', cpu: null }] },
  ],
});

assertContains('renderSystem cpu card', dom.system.innerHTML, '25.5%');
assertContains('renderSystem ram card', dom.system.innerHTML, '8.0 GB');
assertContains('renderSystem ram total', dom.system.innerHTML, 'of 16.0 GB');
assertContains('renderSystem disk', dom.system.innerHTML, 'R 1.0 KB/s');
assertContains('renderSystem disk write', dom.system.innerHTML, 'W 2.0 KB/s');
assertContains('renderSystem net', dom.system.innerHTML, 'RX 512 B/s');
assertContains('renderSystem temp label', dom.system.innerHTML, 'Tctl');
assertContains('renderSystem temp value', dom.system.innerHTML, '55.0°C');
assertContains('renderSystem gpu', dom.system.innerHTML, 'busy 80.0%');
assertContains('renderSystem card markup', dom.system.innerHTML, 'class="card-label"');

// The meter is four segments: half-lit for a partially covered segment, so a
// 50% reading must fill exactly two and leave the rest dark.
assertEq('meter 50%', helpers.meter(50),
  '<div class="meter"><span class="on"></span><span class="on"></span><span></span><span></span></div>');
assertEq('meter absent', helpers.meter(null), '');

assertContains('renderProcesses heading', dom.processes.innerHTML, 'Programs &amp; Processes');
assertContains('renderProcesses first row sorted by RAM', dom.processes.innerHTML, '<tr data-pid="42">');
assertContains('renderProcesses name', dom.processes.innerHTML, 'browser');
assertContains('renderProcesses disk io', dom.processes.innerHTML, 'R 100 B/s');
assertContains('renderProcesses kill button', dom.processes.innerHTML, 'data-kill-pid="42"');
assertContains('renderProcesses sort control', dom.processes.innerHTML, 'data-sort-select');
// The biggest process anchors the bar scale at 100%.
assertContains('renderProcesses ram bar', dom.processes.innerHTML, '<i style="width:100%"></i>');

assertContains('renderDocker project', dom.docker.innerHTML, 'core');
assertContains('renderDocker stack row', dom.docker.innerHTML, 'data-stack="core"');
assertContains('renderDocker stack expanded', dom.docker.innerHTML, 'aria-expanded="true"');
assertContains('renderDocker child linked to stack', dom.docker.innerHTML, 'data-stack-child="core"');
assertContains('renderDocker container id', dom.docker.innerHTML, 'data-container-id="c1"');
assertContains('renderDocker ungrouped', dom.docker.innerHTML, '(ungrouped)');
assertContains('renderDocker stop button', dom.docker.innerHTML, 'data-container-stop="c1"');
assertContains('renderDocker restart button', dom.docker.innerHTML, 'data-container-restart="c1"');
// The stack row aggregates its containers rather than repeating a single one.
assertContains('renderDocker stack aggregate ram', dom.docker.innerHTML, '128.0 MB');

const killBtn = helpers.killButton(123);
assertContains('killButton has pid', killBtn, 'data-kill-pid="123"');
assertContains('killButton confirms', killBtn, 'kill 123?');

const containerBtn = helpers.containerButtons('abc123');
assertContains('containerButtons stop', containerBtn, 'data-container-stop="abc123"');
assertContains('containerButtons restart', containerBtn, 'data-container-restart="abc123"');
assertContains('containerButtons stop confirm', containerBtn, 'stop abc123?');
assertContains('containerButtons restart confirm', containerBtn, 'restart abc123?');

// A full 64-char id must reach the action endpoints intact but never be shown.
const longID = '0123456789abcdef'.repeat(4);
const longBtn = helpers.containerButtons(longID);
assertContains('containerButtons keeps full id', longBtn, 'data-container-stop="' + longID + '"');
assertContains('containerButtons shows short id', longBtn, 'stop 0123456789ab? ');

const byCpu = helpers.sortProcesses([
  { pid: 1, cpu: 5, ram: { used: 100 } },
  { pid: 2, cpu: 50, ram: { used: 50 } },
  { pid: 3 },
], 'cpu');
assertEq('sortByCpu first', byCpu[0].pid, 2);
assertEq('sortByCpu last', byCpu[2].pid, 3);

const byDisk = helpers.sortProcesses([
  { pid: 1, disk_io: { read: 10, write: 10 } },
  { pid: 2, disk_io: { read: 100, write: 200 } },
  { pid: 3, ram: { used: 1e9 } },
], 'disk_io');
assertEq('sortByDisk first', byDisk[0].pid, 2);
assertEq('sortByDisk last', byDisk[2].pid, 3);

if (failures) {
  console.error('\n' + failures + ' test(s) failed');
  process.exit(1);
}
console.log('OK — ' + appPath + ' frontend render tests passed');
