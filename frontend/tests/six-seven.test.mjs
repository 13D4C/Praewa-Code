import test from 'node:test';
import { pose } from './pose-fixture.mjs';
import assert from 'node:assert/strict';
import {
  SixSevenCounter,
  PromptChallenge,
  SIX_SEVEN_TARGET,
} from '../src/lib/six-seven.ts';
import {
  CameraLease,
  webcamDevices,
  preferredWebcam,
  webcamConstraints,
} from '../src/lib/gesture-camera.ts';

function exercise(counter) {
  let time = 0;
  let progress;
  return {
    hold(points, frames = 8) {
      for (let frame = 0; frame < frames; frame++) {
        time += 50;
        progress = counter.update(points, time);
      }
      return progress;
    },
    cycle() {
      this.hold(pose(0.6, 0.4));
      return this.hold(pose(0.4, 0.6));
    },
    gap() {
      time += 500;
    },
  };
}

test('requires 50 full alternating cycles before a prompt can be consumed exactly once', () => {
  const counter = new SixSevenCounter();
  const gate = new PromptChallenge();
  const run = exercise(counter);
  const prompt = { text: 'Build the feature', mode: 'frog' };
  gate.begin('first', prompt);
  run.hold(pose(0.4, 0.6));
  for (let count = 1; count < 50; count++) {
    const result = run.cycle();
    assert.equal(result.count, count);
    assert.equal(gate.complete('first', result.count), null);
  }
  assert.equal(run.hold(pose(0.6, 0.4)).count, 49);
  const result = run.hold(pose(0.4, 0.6));
  assert.equal(result.count, SIX_SEVEN_TARGET);
  assert.equal(gate.complete('first', result.count), prompt);
  assert.equal(gate.complete('first', result.count), null);
  for (let i = 0; i < 5; i++) assert.equal(run.cycle().count, 50);
});

test('new prompts, cancellation and stale completions cannot reuse a completed challenge', () => {
  const gate = new PromptChallenge();
  gate.begin('old', 'old prompt');
  gate.cancel();
  assert.equal(gate.complete('old', 50), null);
  gate.begin('new', 'new prompt');
  assert.equal(gate.complete('old', 50), null);
  assert.equal(gate.complete('new', 49), null);
  assert.equal(gate.complete('new', 51), null);
  assert.equal(gate.complete('new', 50), 'new prompt');
  const counter = new SixSevenCounter();
  const run = exercise(counter);
  run.hold(pose(0.4, 0.6));
  assert.equal(run.cycle().count, 1);
  counter.reset();
  assert.equal(counter.update(pose(0.4, 0.6), 1).count, 0);
});

test('stationary hands, camera jitter, body bouncing and one moving arm do not count', () => {
  for (const kind of ['stationary', 'jitter', 'bounce', 'one-arm']) {
    const counter = new SixSevenCounter();
    const run = exercise(counter);
    let result;
    for (let index = 0; index < 100; index++) {
      const flip = index % 2;
      const points =
        kind === 'stationary'
          ? pose(0.4, 0.6)
          : kind === 'jitter'
            ? pose(0.49 + flip * 0.02, 0.51 - flip * 0.02)
            : kind === 'bounce'
              ? pose(0.4, 0.6, flip * 0.08)
              : pose(flip ? 0.65 : 0.35, 0.5);
      result = run.hold(points);
    }
    assert.equal(result.count, 0, kind);
  }
});

test('lost, occluded, offscreen or interrupted frames discard partial cycles without adding a rep', () => {
  for (const interruption of [
    'missing',
    'low-confidence',
    'offscreen',
    'nan',
    'gap',
  ]) {
    const counter = new SixSevenCounter();
    const run = exercise(counter);
    run.hold(pose(0.4, 0.6));
    assert.equal(run.cycle().count, 1);
    assert.equal(run.hold(pose(0.6, 0.4)).halfway, true);
    const invalid = pose(0.6, 0.4);
    if (interruption === 'missing') run.hold(null);
    else if (interruption === 'gap') run.gap();
    else {
      if (interruption === 'low-confidence') invalid[15].visibility = 0.1;
      if (interruption === 'offscreen') invalid[16].x = 1.1;
      if (interruption === 'nan') invalid[15].y = NaN;
      run.hold(invalid);
    }
    assert.equal(run.hold(pose(0.4, 0.6)).count, 1, interruption);
    assert.equal(run.cycle().count, 2, interruption);
  }
});

test('cancelled camera requests stop streams that arrive after cancellation', async () => {
  let resolve;
  let stops = 0;
  const stream = { getTracks: () => [{ stop: () => stops++ }] };
  const lease = new CameraLease();
  const pending = lease.acquire(
    () =>
      new Promise((done) => {
        resolve = done;
      }),
  );
  lease.stop();
  resolve(stream);
  assert.equal(await pending, null);
  assert.equal(stops, 1);
  let requested = false;
  assert.equal(
    await lease.acquire(async () => {
      requested = true;
      return stream;
    }),
    null,
  );
  assert.equal(requested, false);
});

test('short gestures count even when the hands never cross the same height', () => {
  const counter = new SixSevenCounter();
  const run = exercise(counter);
  run.hold(pose(0.35, 0.72));
  for (let count = 1; count <= 12; count++) {
    assert.equal(run.hold(pose(0.42, 0.65)).halfway, true);
    assert.equal(run.hold(pose(0.35, 0.72)).count, count);
  }
});

test('natural continuous movement counts consistently at different frame rates and distances', () => {
  for (const fps of [8, 12, 20, 30]) {
    for (const period of [600, 1000, 1800]) {
      for (const scale of [0.7, 1, 1.8]) {
        const counter = new SixSevenCounter();
        const interval = 1000 / fps;
        const amplitude = 0.065 * scale;
        let progress;
        for (let time = 0; time <= period * 8; time += interval) {
          const offset = amplitude * Math.cos((time * 2 * Math.PI) / period);
          const frame = pose(0.42 - offset, 0.6 + offset, 0, scale);
          progress = counter.update(frame, time, 16 / 9);
        }
        for (let frame = 1; frame <= 4; frame++)
          progress = counter.update(
            pose(0.42 - amplitude, 0.6 + amplitude, 0, scale),
            period * 8 + frame * interval,
            16 / 9,
          );
        assert.equal(progress.count, 8, JSON.stringify({ fps, period, scale }));
      }
    }
  }
});

test('each wrist adapts to its arm scale', () => {
  const counter = new SixSevenCounter();
  const run = exercise(counter);
  const start = pose(0.35, 0.65);
  start[11].y = 0.1;
  start[12].y = 0.45;
  const turn = structuredClone(start);
  turn[15].y = 0.49;
  turn[16].y = 0.61;
  run.hold(start);
  for (let count = 1; count <= 8; count++) {
    run.hold(turn);
    assert.equal(run.hold(start).count, count);
  }
});

test('a brief missing frame pauses counting without losing the current half cycle', () => {
  const counter = new SixSevenCounter();
  const run = exercise(counter);
  run.hold(pose(0.4, 0.6));
  assert.equal(run.hold(pose(0.6, 0.4)).halfway, true);
  const missing = run.hold(null, 1);
  assert.equal(missing.tracking, false);
  assert.equal(missing.count, 0);
  assert.equal(run.hold(pose(0.6, 0.4), 2).halfway, true);
  assert.equal(run.hold(pose(0.4, 0.6)).count, 1);
});

test('crossed arms keep their skeleton identities and still count opposing wrist movement', () => {
  const counter = new SixSevenCounter();
  const run = exercise(counter);
  const start = pose(0.4, 0.6);
  const turn = pose(0.6, 0.4);
  start[15].x = turn[15].x = 0.35;
  start[16].x = turn[16].x = 0.65;
  run.hold(start);
  for (let count = 1; count <= 10; count++) {
    run.hold(turn);
    const progress = run.hold(start);
    assert.equal(progress.count, count);
    assert.equal(progress.arms[0].side, 'left');
    assert.equal(progress.arms[0].wrist.x, 0.35);
    assert.equal(progress.arms[1].side, 'right');
  }
});

test('single-frame landmark spikes cannot count as an alternating cycle', () => {
  const counter = new SixSevenCounter();
  const run = exercise(counter);
  run.hold(pose(0.4, 0.6));
  assert.equal(run.cycle().count, 1);
  for (let i = 0; i < 20; i++) {
    run.hold(pose(0.1, 0.9), 1);
    run.hold(pose(0.4, 0.6));
    run.hold(pose(0.7, 0.3), 1);
    assert.equal(run.hold(pose(0.4, 0.6)).count, 1);
  }
});

test('counting needs only the two wrists; occluded elbows, shoulders and fingers do not block it', () => {
  const counter = new SixSevenCounter();
  const run = exercise(counter);
  const start = pose(0.4, 0.6);
  const turn = pose(0.6, 0.4);
  for (const points of [start, turn]) {
    for (let index = 0; index < points.length; index++) {
      if (index !== 15 && index !== 16) points[index].visibility = 0.1;
    }
  }
  assert.equal(run.hold(start).tracking, true);
  for (let count = 1; count <= 8; count++) {
    run.hold(turn);
    assert.equal(run.hold(start).count, count);
  }
  const missing = pose(0.4, 0.6);
  missing[16].visibility = 0.1;
  assert.equal(run.hold(missing).arms.length, 1);
  assert.equal(run.hold(missing).tracking, false);
});

test('camera shutdown releases every track once even with repeated cleanup', async () => {
  let stops = 0;
  const stream = {
    getTracks: () => [1, 2].map(() => ({ stop: () => stops++ })),
  };
  const lease = new CameraLease();
  assert.equal(await lease.acquire(async () => stream), stream);
  lease.stop();
  lease.stop();
  assert.equal(stops, 2);
});

test('webcam selection prefers an explicit color camera over infrared and default aliases', () => {
  const devices = webcamDevices([
    { kind: 'audioinput', deviceId: 'mic', label: 'Microphone' },
    { kind: 'videoinput', deviceId: 'default', label: 'Default camera' },
    { kind: 'videoinput', deviceId: 'ir', label: 'USB IR Camera' },
    { kind: 'videoinput', deviceId: 'rgb', label: 'USB FHD Webcam' },
    { kind: 'videoinput', deviceId: 'rgb', label: 'USB FHD Webcam' },
  ]);
  assert.equal(devices.length, 3);
  assert.equal(preferredWebcam(devices), 'rgb');
  assert.equal(preferredWebcam(devices, 'ir'), 'ir');
  assert.equal(preferredWebcam(devices, 'disconnected'), 'rgb');
  assert.equal(
    preferredWebcam([
      { deviceId: 'first', label: 'USB FHD Webcam' },
      { deviceId: 'second', label: 'USB FHD Webcam' },
    ]),
    'first',
  );
  assert.deepEqual(webcamConstraints('rgb').video.deviceId, { exact: 'rgb' });
  assert.equal(webcamConstraints('rgb').audio, false);
  assert.equal(webcamConstraints('rgb').video.frameRate.max, undefined);
  assert.equal(webcamConstraints().video.facingMode, undefined);
});

test('switching webcams releases the previous stream and ignores superseded requests', async () => {
  const lease = new CameraLease();
  let stops = 0;
  const stream = () => ({ getTracks: () => [{ stop: () => stops++ }] });
  const first = stream();
  await lease.acquire(async () => first);
  let resolve;
  const stale = lease.acquire(
    () =>
      new Promise((done) => {
        resolve = done;
      }),
  );
  assert.equal(stops, 1);
  const next = stream();
  assert.equal(await lease.acquire(async () => next), next);
  resolve(stream());
  assert.equal(await stale, null);
  assert.equal(stops, 2);
  lease.stop();
  assert.equal(stops, 3);
});
