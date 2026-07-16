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
  system: { innerHTML: '', textContent: '' },
  processes: { innerHTML: '' },
  docker: { innerHTML: '' },
};

global.document = {
  readyState: 'complete',
  getElementById: function (id) {
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
  '\nreturn { escapeHtml: escapeHtml, fmtPercent: fmtPercent, fmtTemp: fmtTemp, fmtBytes: fmtBytes, fmtRate: fmtRate, sortByRamDesc: sortByRamDesc, renderSystem: renderSystem, renderProcesses: renderProcesses, renderDocker: renderDocker, render: render };' +
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

assertContains('renderSystem cpu', dom.system.innerHTML, '25.5%');
assertContains('renderSystem ram', dom.system.innerHTML, '8.0 GB / 16.0 GB');
assertContains('renderSystem disk', dom.system.innerHTML, 'R 1.0 KB/s / W 2.0 KB/s');
assertContains('renderSystem net', dom.system.innerHTML, 'RX 512 B/s / TX 256 B/s');
assertContains('renderSystem temp', dom.system.innerHTML, 'Tctl: 55.0°C');
assertContains('renderSystem gpu', dom.system.innerHTML, 'busy 80.0%');

assertContains('renderProcesses heading', dom.processes.innerHTML, 'Processes');
assertContains('renderProcesses first row sorted by RAM', dom.processes.innerHTML, '<tr data-pid="42">');
assertContains('renderProcesses name', dom.processes.innerHTML, 'browser');
assertContains('renderProcesses disk io', dom.processes.innerHTML, 'R 100 B/s');

assertContains('renderDocker project', dom.docker.innerHTML, 'core');
assertContains('renderDocker container id', dom.docker.innerHTML, 'data-container-id="c1"');
assertContains('renderDocker ungrouped', dom.docker.innerHTML, '(ungrouped)');

if (failures) {
  console.error('\n' + failures + ' test(s) failed');
  process.exit(1);
}
console.log('OK — ' + appPath + ' frontend render tests passed');
