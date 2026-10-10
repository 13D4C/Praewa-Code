import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { pathToFileURL } from 'node:url';
import { build } from 'esbuild';

const directory = await mkdtemp(join(tmpdir(), 'praewa-camera-test-'));
const bundle = join(directory, 'camera.mjs');
await build({
  entryPoints: ['src/lib/gesture-camera.ts'],
  bundle: true,
  format: 'esm',
  outfile: bundle,
  plugins: [
    {
      name: 'vision-fixture',
      setup(builder) {
        builder.onResolve({ filter: /^@mediapipe\/tasks-vision$/ }, () => ({
          path: 'vision',
          namespace: 'fixture',
        }));
        builder.onLoad({ filter: /.*/, namespace: 'fixture' }, () => ({
          contents: `
      export const FilesetResolver = {forVisionTasks: async () => ({})};
      export const PoseLandmarker = {createFromOptions: async () => globalThis.cameraFixture.detector()};
    `,
        }));
      },
    },
  ],
});
const { gestureCamera } = await import(pathToFileURL(bundle).href);

function fixture({
  worker = false,
  unsupported = false,
  latency = 65,
  open,
  enumerate,
  play,
  deviceId = '',
  native = false,
  nativeOpen,
  nativeFrame,
} = {}) {
  const keys = [
    'window',
    'document',
    'navigator',
    'performance',
    'requestAnimationFrame',
    'cancelAnimationFrame',
    'setTimeout',
    'clearTimeout',
    'setInterval',
    'clearInterval',
    'OffscreenCanvas',
    'createImageBitmap',
    'Worker',
    'cameraFixture',
  ];
  const saved = new Map(
    keys.map((key) => [key, Object.getOwnPropertyDescriptor(globalThis, key)]),
  );
  const intervals = new Map();
  const rafs = new Map(),
    timers = new Map();
  let clock = 0,
    id = 0,
    closes = 0,
    stops = 0,
    workers = 0,
    terminations = 0,
    bitmapCloses = 0,
    bitmapResolve;
  const detections = [],
    messages = [],
    access = [],
    errors = [],
    previews = [],
    requests = [],
    listed = [];
  const nativeClosed = [],
    nativeRequests = [],
    sources = [];
  let nativeReads = 0,
    decodes = 0;
  const image = {
    src: '',
    complete: true,
    naturalWidth: 640,
    naturalHeight: 480,
    decode: async () => {
      decodes++;
    },
    removeAttribute: () => {
      image.src = '';
    },
  };
  const track = {
    stop: () => stops++,
    getSettings: () => ({ deviceId: 'rgb' }),
    addEventListener: () => {},
  };
  const stream = { getTracks: () => [track], getVideoTracks: () => [track] };
  const video = {
    readyState: 4,
    videoWidth: 1920,
    videoHeight: 1080,
    currentTime: 0,
    srcObject: null,
    play: play ?? (async () => {}),
  };
  const detector = {
    close: () => closes++,
    detectForVideo(input, timestamp) {
      detections.push({ timestamp, width: input.width, height: input.height });
      clock += latency;
      return { landmarks: [[]] };
    },
  };
  const set = (key, value) =>
    Object.defineProperty(globalThis, key, {
      configurable: true,
      writable: true,
      value,
    });
  set('cameraFixture', { detector: async () => detector });
  set('window', {
    location: { href: 'https://praewa.test/' },
    go: {
      main: {
        App: {
          CameraAccess: async (enabled) => access.push(enabled),
          ...(native
            ? {
                CameraDevices: async () => ({
                  native: true,
                  devices: [{ deviceId: '/dev/video0', label: 'Webcam' }],
                  error: '',
                }),
                CameraOpen: async (id, device) => {
                  nativeRequests.push({ id, device });
                  return nativeOpen ? nativeOpen(device) : device;
                },
                CameraFrame: async () => {
                  nativeReads++;
                  return nativeFrame
                    ? nativeFrame()
                    : {
                        sequence: 1,
                        image: 'data:image/jpeg;base64,fixture',
                        error: '',
                      };
                },
                CameraClose: async (id) => {
                  nativeClosed.push(id);
                },
              }
            : {}),
        },
      },
    },
  });
  set('document', {
    hidden: false,
    createElement: () => ({
      width: 0,
      height: 0,
      getContext: () => ({
        drawImage: (source) => sources.push(source),
        getImageData: (_, __, width, height) => ({
          width,
          height,
          data: new Uint8ClampedArray(4),
        }),
      }),
    }),
  });
  set('navigator', {
    mediaDevices: {
      getUserMedia: async (constraints) => {
        requests.push(constraints);
        return open ? open(stream) : stream;
      },
      enumerateDevices:
        enumerate ??
        (async () => [
          { kind: 'videoinput', deviceId: 'rgb', label: 'Webcam' },
        ]),
    },
  });
  set('performance', { now: () => clock });
  set('requestAnimationFrame', (callback) => {
    rafs.set(++id, callback);
    return id;
  });
  set('cancelAnimationFrame', (token) => rafs.delete(token));
  set('setTimeout', (callback, delay) => {
    timers.set(++id, { callback, due: clock + delay });
    return id;
  });
  set('clearTimeout', (token) => timers.delete(token));
  set('setInterval', (callback) => {
    intervals.set(++id, callback);
    return id;
  });
  set('clearInterval', (token) => intervals.delete(token));
  set('OffscreenCanvas', worker ? class {} : undefined);
  set(
    'createImageBitmap',
    worker
      ? () =>
          new Promise((resolve) => {
            bitmapResolve = resolve;
          })
      : undefined,
  );
  set(
    'Worker',
    class {
      constructor() {
        workers++;
      }
      postMessage(message) {
        messages.push(message);
        if (message.type === 'init')
          queueMicrotask(() =>
            this.onmessage({
              data: unsupported
                ? {
                    type: 'error',
                    unsupported: true,
                    message: 'No worker WebGL',
                  }
                : { type: 'ready' },
            }),
          );
      }
      terminate() {
        terminations++;
      }
    },
  );
  const camera = gestureCamera(
    video,
    () => {},
    () => {},
    (error) => errors.push(error),
    {
      deviceId,
      ...(native ? { image } : {}),
      onpreview: (...size) => previews.push(size),
      ondevices: (devices, active) => listed.push({ devices, active }),
    },
  );
  return {
    camera,
    video,
    detections,
    errors,
    access,
    messages,
    previews,
    requests,
    listed,
    image,
    sources,
    nativeClosed,
    nativeRequests,
    get nativeReads() {
      return nativeReads;
    },
    get decodes() {
      return decodes;
    },
    get closes() {
      return closes;
    },
    get stops() {
      return stops;
    },
    get workers() {
      return workers;
    },
    get terminations() {
      return terminations;
    },
    get bitmapCloses() {
      return bitmapCloses;
    },
    async raf(time) {
      clock = time;
      video.currentTime = time / 1000;
      const callbacks = [...rafs.values()];
      rafs.clear();
      for (const callback of callbacks) await callback(clock);
    },
    runInference() {
      const item = [...timers.entries()].sort((a, b) => a[1].due - b[1].due)[0];
      assert.ok(item, 'An inference should be queued');
      timers.delete(item[0]);
      clock = Math.max(clock, item[1].due);
      item[1].callback();
    },
    async advance(time) {
      clock = time;
      const due = [...timers.entries()].filter(
        ([, timer]) => timer.due <= time,
      );
      for (const [token, timer] of due) {
        if (!timers.delete(token)) continue;
        timer.callback();
      }
      for (let pass = 0; pass < 15; pass++) await Promise.resolve();
    },
    finishBitmap() {
      bitmapResolve({ close: () => bitmapCloses++ });
    },
    async poll() {
      for (const callback of intervals.values()) callback();
      for (let pass = 0; pass < 15; pass++) await Promise.resolve();
    },
    async restore() {
      camera.stop();
      await Promise.resolve();
      await Promise.resolve();
      for (const [key, descriptor] of saved) {
        if (descriptor) Object.defineProperty(globalThis, key, descriptor);
        else delete globalThis[key];
      }
    },
  };
}

test('slow CPU inference yields to rendering, downsizes HD frames and leaves idle time', async () => {
  const run = fixture();
  try {
    await run.camera.start();
    await run.raf(100);
    assert.equal(
      run.detections.length,
      0,
      'Inference must not run inside the animation callback',
    );
    run.runInference();
    assert.deepEqual(run.detections[0], {
      timestamp: 116,
      width: 640,
      height: 360,
    });
    for (const time of [182, 198, 214]) await run.raf(time);
    assert.equal(run.detections.length, 1);
    await run.raf(216);
    run.runInference();
    assert.equal(run.detections.length, 2);
    assert.ok(run.detections[1].timestamp - run.detections[0].timestamp >= 100);
    assert.equal(run.errors.length, 0);
    run.camera.stop();
    assert.equal(run.closes, 1);
    assert.equal(run.stops, 1);
  } finally {
    await run.restore();
  }
});

test('a webcam request that never resolves times out and releases a late stream', async () => {
  let resolve;
  const run = fixture({
    open: (stream) =>
      new Promise((done) => {
        resolve = () => done(stream);
      }),
  });
  try {
    const starting = run.camera.start();
    await run.advance(0);
    assert.equal(run.requests.length, 1);
    await run.advance(10000);
    await starting;
    assert.match(run.errors[0].message, /10/);
    assert.equal(run.previews.length, 0);
    resolve();
    await run.advance(10001);
    assert.equal(run.stops, 1);
    assert.equal(run.video.srcObject, null);
  } finally {
    await run.restore();
  }
});

test('cancelling while a webcam is opening settles startup and closes its late result', async () => {
  let resolve;
  const run = fixture({
    open: (stream) =>
      new Promise((done) => {
        resolve = () => done(stream);
      }),
  });
  try {
    const starting = run.camera.start();
    await run.advance(0);
    run.camera.stop();
    await starting;
    resolve();
    await run.advance(1);
    assert.equal(run.stops, 1);
    assert.equal(run.errors.length, 0);
    assert.equal(run.previews.length, 0);
  } finally {
    await run.restore();
  }
});

test('a stalled device list cannot hold back preview or detection and never reopens the webcam', async () => {
  const run = fixture({ enumerate: () => new Promise(() => {}) });
  try {
    await run.camera.start();
    assert.deepEqual(run.previews, [[1920, 1080]]);
    assert.equal(run.requests.length, 1);
    assert.equal(run.listed[0].active, 'rgb');
    await run.advance(2500);
    await run.raf(2600);
    run.runInference();
    assert.equal(run.detections.length, 1);
    assert.equal(run.errors.length, 0);
  } finally {
    await run.restore();
  }
});

test('the device list keeps the camera already in use instead of reopening a different default', async () => {
  const run = fixture({
    enumerate: async () => [
      { kind: 'videoinput', deviceId: 'other', label: 'Other webcam' },
    ],
  });
  try {
    await run.camera.start();
    await run.advance(0);
    assert.equal(run.requests.length, 1);
    assert.equal(run.listed.at(-1).active, 'rgb');
    assert.deepEqual(
      run.listed.at(-1).devices.map((device) => device.deviceId),
      ['rgb', 'other'],
    );
  } finally {
    await run.restore();
  }
});

test('a webcam that opens but never plays times out and releases its tracks', async () => {
  const run = fixture({ play: () => new Promise(() => {}) });
  try {
    const starting = run.camera.start();
    await run.advance(0);
    await run.advance(8000);
    await starting;
    assert.equal(run.errors.length, 1);
    assert.equal(run.stops, 1);
    assert.equal(run.previews.length, 0);
  } finally {
    await run.restore();
  }
});

test('cancelling a queued CPU frame releases resources without running more inference', async () => {
  const run = fixture();
  try {
    await run.camera.start();
    await run.raf(100);
    run.camera.stop();
    await run.raf(200);
    assert.equal(run.detections.length, 0);
    assert.equal(run.closes, 1);
    assert.equal(run.stops, 1);
    assert.equal(run.video.srcObject, null);
  } finally {
    await run.restore();
  }
});

test('a late bitmap is closed after cancellation and is never sent to the worker', async () => {
  const run = fixture({ worker: true });
  try {
    await run.camera.start();
    const frame = run.raf(100);
    await Promise.resolve();
    run.camera.stop();
    run.finishBitmap();
    await frame;
    assert.equal(run.bitmapCloses, 1);
    assert.equal(
      run.messages.filter((message) => message.type === 'frame').length,
      0,
    );
    assert.equal(run.terminations, 1);
    assert.equal(run.stops, 1);
  } finally {
    await run.restore();
  }
});

test('desktop capture previews native frames without requesting WebKit camera access', async () => {
  const run = fixture({ native: true });
  try {
    await run.camera.start();
    assert.equal(run.requests.length, 0);
    assert.equal(run.nativeRequests.length, 1);
    assert.deepEqual(run.previews, [[640, 480]]);
    assert.equal(run.access.includes(true), false);
    await run.raf(100);
    run.runInference();
    assert.equal(run.sources[0], run.image);
    assert.equal(run.detections.length, 1);
    await run.poll();
    assert.equal(run.decodes, 1);
    run.camera.stop();
    await run.advance(200);
    assert.equal(run.nativeClosed.length, 1);
  } finally {
    await run.restore();
  }
});

test('cancelling native startup closes its session and discards a late open', async () => {
  let resolve;
  const run = fixture({
    native: true,
    nativeOpen: (device) =>
      new Promise((done) => {
        resolve = () => done(device);
      }),
  });
  try {
    const starting = run.camera.start();
    await run.advance(0);
    assert.equal(run.nativeRequests.length, 1);
    run.camera.stop();
    await starting;
    resolve();
    await run.advance(1);
    assert.equal(run.nativeClosed.length, 1);
    assert.equal(run.nativeReads, 0);
    assert.equal(run.previews.length, 0);
    assert.equal(run.errors.length, 0);
  } finally {
    await run.restore();
  }
});

test('native capture failure closes the helper and reports an inline error', async () => {
  const run = fixture({
    native: true,
    nativeFrame: async () => ({ sequence: 0, image: '', error: 'Camera busy' }),
  });
  try {
    await run.camera.start();
    assert.equal(run.errors.length, 1);
    assert.match(run.errors[0].message, /Camera busy/);
    assert.equal(run.nativeClosed.length, 1);
    assert.equal(run.requests.length, 0);
  } finally {
    await run.restore();
  }
});

test('native frame requests stay bounded when the capture helper is slow', async () => {
  let resolve;
  const run = fixture({
    native: true,
    nativeFrame: () =>
      new Promise((done) => {
        resolve = done;
      }),
  });
  try {
    const starting = run.camera.start();
    await run.advance(0);
    for (let attempt = 0; attempt < 10; attempt++) await run.poll();
    assert.equal(run.nativeReads, 1);
    resolve({
      sequence: 1,
      image: 'data:image/jpeg;base64,fixture',
      error: '',
    });
    await starting;
    assert.equal(run.decodes, 1);
    assert.equal(run.previews.length, 1);
  } finally {
    await run.restore();
  }
});

test('unsupported worker graphics falls back once and later cameras skip the failed path', async () => {
  for (let attempt = 0; attempt < 2; attempt++) {
    const run = fixture({ worker: true, unsupported: true });
    try {
      await run.camera.start();
      await run.raf(100);
      run.runInference();
      assert.equal(run.detections.length, 1);
      assert.equal(run.workers, attempt === 0 ? 1 : 0);
      assert.equal(run.errors.length, 0);
    } finally {
      await run.restore();
    }
  }
});

test.after(async () => {
  await rm(directory, { recursive: true, force: true });
});
