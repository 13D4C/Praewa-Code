import test from 'node:test';
import { pose } from './pose-fixture.mjs';
import assert from 'node:assert/strict';
import { readFile, mkdtemp, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join, resolve } from 'node:path';
import { pathToFileURL } from 'node:url';
import { build } from 'esbuild';
import { compile } from 'svelte/compiler';
import { JSDOM } from 'jsdom';

const dom = new JSDOM('<!doctype html><body></body>', {
  url: 'https://praewa.test/',
  pretendToBeVisual: true,
});
for (const key of [
  'window',
  'document',
  'Node',
  'Text',
  'Comment',
  'Element',
  'HTMLElement',
  'SVGElement',
  'HTMLInputElement',
  'HTMLTextAreaElement',
  'CustomEvent',
  'Event',
  'MouseEvent',
  'MutationObserver',
  'localStorage',
])
  globalThis[key] = dom.window[key];
const directory = await mkdtemp(join(tmpdir(), 'praewa-panel-test-'));
const bundle = join(directory, 'panel.mjs');
await build({
  stdin: {
    contents:
      "import Panel from './src/components/AgentPanel.svelte'; export { Panel }; export { default as Challenge } from './src/components/SixSevenChallenge.svelte'; export { mount, tick, unmount } from 'svelte';",
    resolveDir: process.cwd(),
  },
  bundle: true,
  format: 'esm',
  conditions: ['browser'],
  outfile: bundle,
  plugins: [
    {
      name: 'panel-fixture',
      setup(builder) {
        builder.onLoad({ filter: /gesture-camera\.ts$/ }, () => ({
          contents: `
        export function gestureCamera(video, onpose, onstatus, onerror, options = {}) {
          globalThis.poseFrame = onpose;
          globalThis.cameraFailure = onerror;
          let stopped = false;
          return {
            async start() {
              globalThis.cameraStarts++;
              options.ondevices?.([{deviceId:'rgb',label:'USB FHD Webcam'},{deviceId:'alternate',label:'USB FHD Webcam'}], options.deviceId || 'rgb');
              options.onpreview?.(640,360);
            },
            stop() { if (!stopped) { stopped = true; globalThis.cameraStops++; } }
          };
        }
        export function cameraError(reason) { return String(reason); }
      `,
        }));
        builder.onLoad({ filter: /file-icons\.ts$/ }, () => ({
          contents: 'export function fileIcon() { return undefined; }',
        }));
        builder.onLoad({ filter: /\.svelte$/ }, async ({ path }) => ({
          contents: compile(await readFile(path, 'utf8'), {
            filename: path,
            generate: 'client',
            css: 'external',
          }).js.code,
          resolveDir: resolve(path, '..'),
        }));
        builder.onResolve({ filter: /\?raw$/ }, ({ path, resolveDir }) => ({
          path: resolve(resolveDir, path.slice(0, -4)),
          namespace: 'raw',
        }));
        builder.onLoad(
          { filter: /.*/, namespace: 'raw' },
          async ({ path }) => ({
            contents: await readFile(path, 'utf8'),
            loader: 'text',
          }),
        );
      },
    },
  ],
});
const { Panel, Challenge, mount, tick, unmount } = await import(
  pathToFileURL(bundle).href
);
let sends = [];
let reads = 0;
let rejectSend = false;
globalThis.cameraStarts = 0;
globalThis.cameraStops = 0;
window.go = {
  main: {
    App: {
      AgentConnect: async () => ({ connected: true, error: '' }),
      AgentStatus: async () => ({ connected: true, error: '' }),
      AgentSnapshot: async () => ({
        thread: { id: 'thread' },
        messages: [],
        requests: [],
        sequence: 1,
        active: false,
      }),
      AgentSend: async (request) => {
        sends.push(request);
        if (rejectSend) throw new Error('Simulated backend failure');
        return {
          conversation: {
            thread: { id: 'thread' },
            messages: [],
            requests: [],
            sequence: 1,
            active: false,
          },
          error: '',
        };
      },
    },
  },
};
const agentEvents = new Set();
window.runtime = {
  EventsOn: (name, callback) => {
    if (name === 'agent:event') agentEvents.add(callback);
    return () => agentEvents.delete(callback);
  },
};

async function settle() {
  await tick();
  await new Promise((done) => setTimeout(done, 0));
  await tick();
}

function input(text) {
  const composer = document.querySelector('textarea');
  composer.value = text;
  composer.dispatchEvent(new Event('input', { bubbles: true }));
}

function click(label) {
  const button = document.querySelector(`[aria-label="${label}"]`);
  assert.ok(button, label);
  button.click();
}

function exercise() {
  let time = 0;
  function hold(left, right) {
    for (let frame = 0; frame < 8; frame++) {
      time += 50;
      globalThis.poseFrame(pose(left, right), time);
    }
  }
  hold(0.4, 0.6);
  return (cycles) => {
    for (let cycle = 0; cycle < cycles; cycle++) {
      hold(0.6, 0.4);
      hold(0.4, 0.6);
    }
  };
}

test('Agent panel gates real send and file reads until 50 camera repetitions, for every prompt', async () => {
  localStorage.clear();
  const panel = mount(Panel, {
    target: document.body,
    props: {
      workspace: '/fixture',
      workspaceName: 'Fixture',
      files: [{ name: 'main.go', path: '/fixture/main.go' }],
      activeFile: { name: 'main.go', path: '/fixture/main.go' },
      visible: true,
      onclose: () => {},
      onreadfile: async () => {
        reads++;
        return 'package main';
      },
    },
  });
  try {
    await settle();
    click('Attach active file main.go');
    input('Build the feature');
    await settle();
    click('Send message to agent');
    await settle();
    assert.ok(document.querySelector('[aria-label="Six Seven challenge"]'));
    assert.equal(cameraStarts, 1);
    assert.equal(sends.length, 0);
    assert.equal(reads, 0);
    exercise()(8);
    await settle();
    const oldCameraFrame = globalThis.poseFrame;
    const cameraSelector = document.querySelector(
      '[aria-label="เลือกเว็บแคม"]',
    );
    assert.ok(cameraSelector);
    assert.match(cameraSelector.options[0].textContent, /· 1/);
    assert.match(cameraSelector.options[1].textContent, /· 2/);
    cameraSelector.value = 'alternate';
    cameraSelector.dispatchEvent(new Event('change', { bubbles: true }));
    await settle();
    assert.equal(cameraStarts, 2);
    assert.equal(
      document
        .querySelector('[role="progressbar"]')
        .getAttribute('aria-valuenow'),
      '0',
    );
    oldCameraFrame(pose(0.6, 0.4), 999999);
    await settle();
    assert.equal(sends.length, 0);
    assert.equal(
      document
        .querySelector('[role="progressbar"]')
        .getAttribute('aria-valuenow'),
      '0',
    );
    assert.equal(localStorage.getItem('praewa-six-seven-webcam'), 'alternate');
    const reps = exercise();
    reps(49);
    await settle();
    assert.equal(sends.length, 0);
    assert.equal(reads, 0);
    assert.equal(
      document
        .querySelector('[role="progressbar"]')
        .getAttribute('aria-valuenow'),
      '49',
    );
    reps(1);
    await settle();
    assert.equal(sends.length, 1);
    assert.equal(sends[0].text, 'Build the feature');
    assert.equal(sends[0].files[0].content, 'package main');
    assert.equal(reads, 1);
    assert.equal(cameraStops, 2);
    assert.equal(
      document.querySelector('[aria-label="Six Seven challenge"]'),
      null,
    );
    input('Another request');
    await settle();
    click('Send message to agent');
    await settle();
    assert.equal(
      document
        .querySelector('[role="progressbar"]')
        .getAttribute('aria-valuenow'),
      '0',
    );
    const staleFrame = globalThis.poseFrame;
    exercise()(10);
    click('Cancel Six Seven challenge');
    await settle();
    assert.equal(sends.length, 1);
    assert.equal(document.querySelector('textarea').value, 'Another request');
    staleFrame(pose(0.4, 0.6), 999999);
    await settle();
    assert.equal(sends.length, 1);
    click('Send message to agent');
    await settle();
    exercise()(12);
    click('New agent chat');
    await settle();
    assert.equal(
      document.querySelector('[aria-label="Six Seven challenge"]'),
      null,
    );
    assert.equal(sends.length, 1);
    input('Retry failure');
    await settle();
    click('Send message to agent');
    await settle();
    globalThis.cameraFailure('Camera denied');
    await settle();
    assert.equal(sends.length, 1);
    assert.match(
      document.querySelector('[role="alert"]').textContent,
      /Camera denied/,
    );
    click('Cancel Six Seven challenge');
    await settle();
    click('Send message to agent');
    await settle();
    rejectSend = true;
    exercise()(50);
    await settle();
    assert.equal(sends.length, 2);
    assert.equal(document.querySelector('textarea').value, 'Retry failure');
    click('Send message to agent');
    await settle();
    assert.equal(
      document
        .querySelector('[role="progressbar"]')
        .getAttribute('aria-valuenow'),
      '0',
    );
    assert.equal(sends.length, 2);
  } finally {
    await unmount(panel);
  }
});

test('challenge displays only the arm skeleton and keeps wrist tracking when elbows are hidden', async () => {
  const challenge = mount(Challenge, {
    target: document.body,
    props: {
      oncomplete: () =>
        assert.fail('A stationary pose cannot complete the challenge'),
      oncancel: () => {},
    },
  });
  try {
    await settle();
    globalThis.poseFrame(pose(0.4, 0.6), 50);
    await settle();
    const skeleton = document.querySelector('svg[viewBox="0 0 100 100"]');
    assert.equal(skeleton.querySelectorAll('line').length, 4);
    assert.equal(skeleton.querySelectorAll('circle').length, 6);
    assert.match(document.body.textContent, /พบข้อมือ 2\/2/);
    const occluded = pose(0.4, 0.6);
    for (const index of [11, 12, 13, 14]) occluded[index].visibility = 0.1;
    globalThis.poseFrame(occluded, 100);
    await settle();
    assert.equal(skeleton.querySelectorAll('line').length, 0);
    assert.equal(skeleton.querySelectorAll('circle').length, 2);
    assert.match(document.body.textContent, /พบข้อมือ 2\/2/);
    occluded[15].visibility = 0.1;
    globalThis.poseFrame(occluded, 150);
    await settle();
    assert.match(document.body.textContent, /พบข้อมือ 1\/2/);
    assert.equal(skeleton.nextElementSibling.textContent.trim(), '7');
  } finally {
    await unmount(challenge);
  }
});

test('the adjustable target persists, restarts an active challenge and controls every actual send', async () => {
  localStorage.clear();
  sends = [];
  reads = 0;
  rejectSend = false;
  const props = {
    workspace: '/adjustable-fixture',
    workspaceName: 'Adjustable Fixture',
    files: [{ name: 'main.go', path: '/adjustable-fixture/main.go' }],
    activeFile: { name: 'main.go', path: '/adjustable-fixture/main.go' },
    visible: true,
    onclose: () => {},
    onreadfile: async () => {
      reads++;
      return 'package main';
    },
  };
  const progress = () => document.querySelector('[role="progressbar"]');
  function setTarget(value) {
    const field = document.querySelector('[aria-label="จำนวนรอบ Six Seven"]');
    field.value = String(value);
    field.dispatchEvent(new Event('change', { bubbles: true }));
    return field;
  }
  let panel = mount(Panel, { target: document.body, props });
  try {
    await settle();
    assert.equal(
      document.querySelector('[aria-label="จำนวนรอบ Six Seven"]').value,
      '50',
    );
    setTarget(3);
    await settle();
    assert.equal(localStorage.getItem('praewa-six-seven-target'), '3');
    click('Attach active file main.go');
    input('Custom target');
    await settle();
    click('Send message to agent');
    await settle();
    assert.equal(progress().getAttribute('aria-valuemax'), '3');
    assert.match(document.body.textContent, /ครบ 3 ส่งข้อความอัตโนมัติ/);
    const three = exercise();
    three(2);
    await settle();
    assert.equal(progress().getAttribute('aria-valuenow'), '2');
    assert.equal(sends.length, 0);
    assert.equal(reads, 0);
    three(1);
    await settle();
    assert.equal(sends.length, 1);
    assert.equal(reads, 1);

    input('Change the target during the challenge');
    await settle();
    click('Send message to agent');
    await settle();
    const original = exercise();
    original(1);
    await settle();
    const originalFrame = globalThis.poseFrame;
    const starts = cameraStarts;
    setTarget(3);
    await settle();
    assert.equal(cameraStarts, starts);
    assert.equal(progress().getAttribute('aria-valuenow'), '1');
    setTarget(5);
    await settle();
    assert.equal(cameraStarts, starts + 1);
    assert.equal(progress().getAttribute('aria-valuenow'), '0');
    assert.equal(progress().getAttribute('aria-valuemax'), '5');
    originalFrame(pose(0.4, 0.6), 999999);
    await settle();
    assert.equal(progress().getAttribute('aria-valuenow'), '0');
    assert.equal(sends.length, 1);
    const five = exercise();
    five(4);
    await settle();
    assert.equal(sends.length, 1);
    assert.equal(reads, 1);
    five(1);
    await settle();
    assert.equal(sends.length, 2);

    for (const value of ['', 0, -1, 1.5, 1001]) {
      const field = setTarget(value);
      await settle();
      assert.equal(field.value, '5');
      assert.equal(field.getAttribute('aria-invalid'), 'true');
      assert.match(
        document.querySelector('[role="alert"]').textContent,
        /ใช้จำนวนเต็ม/,
      );
      assert.equal(localStorage.getItem('praewa-six-seven-target'), '5');
    }
    await unmount(panel);
    panel = mount(Panel, {
      target: document.body,
      props: { ...props, workspace: '/another-workspace' },
    });
    await settle();
    assert.equal(
      document.querySelector('[aria-label="จำนวนรอบ Six Seven"]').value,
      '5',
    );
    input('One full repetition');
    await settle();
    click('Send message to agent');
    await settle();
    assert.equal(progress().getAttribute('aria-valuemax'), '5');
    exercise()(4);
    await settle();
    assert.equal(progress().getAttribute('aria-valuenow'), '4');
    setTarget(1);
    await settle();
    assert.equal(progress().getAttribute('aria-valuemax'), '1');
    assert.equal(progress().getAttribute('aria-valuenow'), '0');
    assert.equal(sends.length, 2);
    exercise()(1);
    await settle();
    assert.equal(sends.length, 3);
  } finally {
    await unmount(panel);
  }
});

test.after(async () => {
  await rm(directory, { recursive: true, force: true });
  dom.window.close();
});

test('streaming updates use a bounded DOM, ignore repeated snapshots and render Markdown when finished', async () => {
  const workspace = '/streaming-performance';
  const snapshot = (sequence, text, status = 'inProgress') => ({
    thread: { id: 'stream-performance', name: 'Streaming' },
    sequence,
    requests: [],
    active: status === 'inProgress',
    canStop: status === 'inProgress',
    messages: [
      {
        id: 'reply',
        role: 'assistant',
        turnId: 'turn',
        text,
        status,
        createdAt: 0,
      },
    ],
  });
  localStorage.setItem(
    'praewa-agent-drafts:' + workspace,
    JSON.stringify({
      selectedId: 'saved',
      chats: [
        {
          id: 'saved',
          title: 'Streaming',
          messages: [],
          composer: '',
          files: [],
          modeId: 'khaow-code',
          threadId: 'stream-performance',
        },
      ],
    }),
  );
  const app = window.go.main.App;
  const oldLoad = app.AgentLoad;
  app.AgentLoad = async () => snapshot(1, 'First update');
  const panel = mount(Panel, {
    target: document.body,
    props: {
      workspace,
      workspaceName: 'Performance',
      files: [],
      activeFile: null,
      visible: true,
      onclose: () => {},
      onreadfile: async () => '',
    },
  });
  const emit = (value) => {
    for (const callback of agentEvents)
      callback({ threadId: value.thread.id, snapshot: value });
  };
  try {
    await settle();
    const reply = () => document.querySelector('[aria-label="Agent reply"]');
    assert.match(reply().textContent, /First update/);
    emit(snapshot(1, 'Repeated snapshot must be ignored'));
    emit(snapshot(0, 'Old snapshot must be ignored'));
    await settle();
    assert.match(reply().textContent, /First update/);
    const text = '**Streaming**\n\n' + '- Item\n'.repeat(3000);
    emit(snapshot(2, text));
    await settle();
    assert.ok(
      reply().querySelectorAll('*').length < 20,
      'Streaming should not rebuild thousands of Markdown nodes',
    );
    assert.match(reply().textContent, /\*\*Streaming\*\*/);
    emit(
      snapshot(
        3,
        '**Completed**<script>window.untrusted = true</script>',
        'completed',
      ),
    );
    await settle();
    assert.equal(reply().querySelector('strong').textContent, 'Agent');
    assert.equal(
      reply().querySelector('.markdown-preview strong').textContent,
      'Completed',
    );
    assert.equal(reply().querySelector('script'), null);
    assert.equal(window.untrusted, undefined);
    assert.equal(
      document.querySelector('[aria-label="Stop agent response"]'),
      null,
    );
  } finally {
    await unmount(panel);
    app.AgentLoad = oldLoad;
  }
});
