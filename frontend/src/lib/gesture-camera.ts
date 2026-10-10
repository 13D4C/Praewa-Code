import type { PoseLandmarker } from '@mediapipe/tasks-vision';
import type { PosePoint } from './six-seven';
import type { nativeCamera as createNativeCamera } from './native-camera';

let accessQueue = Promise.resolve();
let workerUnavailable = false;

export type WebcamDevice = { deviceId: string; label: string };

export function webcamDevices(
  devices: Pick<MediaDeviceInfo, 'kind' | 'deviceId' | 'label'>[],
): WebcamDevice[] {
  const seen = new Set<string>();
  return devices
    .filter((device) => {
      if (
        device.kind !== 'videoinput' ||
        !device.deviceId ||
        seen.has(device.deviceId)
      )
        return false;
      seen.add(device.deviceId);
      return true;
    })
    .map((device, index) => ({
      deviceId: device.deviceId,
      label: device.label || `Webcam ${index + 1}`,
    }));
}

export function preferredWebcam(
  devices: WebcamDevice[],
  preferred = '',
): string {
  if (devices.some((device) => device.deviceId === preferred)) return preferred;
  const physical = devices.filter(
    (device) => !['default', 'communications'].includes(device.deviceId),
  );
  const candidates = physical.length ? physical : devices;
  return (
    (
      candidates.find(
        (device) => !/\b(ir|infrared|depth|metadata)\b/i.test(device.label),
      ) ?? candidates[0]
    )?.deviceId ?? ''
  );
}

export function webcamConstraints(deviceId = ''): MediaStreamConstraints {
  return {
    audio: false,
    video: {
      ...(deviceId ? { deviceId: { exact: deviceId } } : {}),
      width: { ideal: 640 },
      height: { ideal: 480 },
      frameRate: { ideal: 24 },
    },
  };
}

function cameraAccess(enabled: boolean) {
  accessQueue = accessQueue
    .catch(() => {})
    .then(async () => {
      await window.go?.main.App.CameraAccess(enabled);
    });
  return accessQueue;
}

export class CameraLease {
  private stopped = false;
  private stream: MediaStream | null = null;
  private generation = 0;

  async acquire(request: () => Promise<MediaStream>) {
    if (this.stopped) return null;
    const generation = ++this.generation;
    this.stream?.getTracks().forEach((track) => track.stop());
    this.stream = null;
    const stream = await request();
    if (this.stopped || generation !== this.generation) {
      stream.getTracks().forEach((track) => track.stop());
      return null;
    }
    this.stream = stream;
    return stream;
  }

  stop() {
    ++this.generation;
    this.stopped = true;
    this.stream?.getTracks().forEach((track) => track.stop());
    this.stream = null;
  }
}

export function gestureCamera(
  video: HTMLVideoElement,
  onpose: (points: PosePoint[] | null, timestamp: number) => void,
  onstatus: (status: string) => void,
  onerror: (reason: unknown) => void,
  options: {
    deviceId?: string;
    ondevices?: (devices: WebcamDevice[], selected: string) => void;
    onpreview?: (width: number, height: number) => void;
    image?: HTMLImageElement;
    onnative?: (enabled: boolean) => void;
  } = {},
) {
  const lease = new CameraLease();
  let stopped = false;
  let worker: Worker | null = null;
  let detector: PoseLandmarker | null = null;
  let native: ReturnType<typeof createNativeCamera> | null = null;
  let animation = 0;
  let inferenceTimer: ReturnType<typeof setTimeout> | undefined;
  let deadline: ReturnType<typeof setTimeout> | undefined;
  let frameDeadline: ReturnType<typeof setTimeout> | undefined;
  let busy = false;
  let recovering = false;
  const lifetime = new AbortController();
  let lastFrame = -1;
  let nextFrameAt = 0;
  const input = document.createElement('canvas');
  let inputContext: CanvasRenderingContext2D | null = null;
  let rejectReady: ((reason: unknown) => void) | undefined;
  const asset = (path: string) =>
    new URL(`/vision/${path}`, window.location.href).href;

  function stop() {
    if (stopped) return;
    stopped = true;
    lifetime.abort();
    clearTimeout(deadline);
    clearTimeout(frameDeadline);
    clearTimeout(inferenceTimer);
    cancelAnimationFrame(animation);
    lease.stop();
    native?.stop();
    video.srcObject = null;
    rejectReady?.(new Error('Camera stopped'));
    worker?.terminate();
    worker = null;
    detector?.close();
    detector = null;
    input.width = input.height = 0;
    void cameraAccess(false).catch(() => {});
  }

  function fail(reason: unknown) {
    if (stopped) return;
    stop();
    onerror(reason);
  }

  function waitFor<T>(operation: Promise<T>, timeout: number, message: string) {
    return new Promise<T>((resolve, reject) => {
      const abort = () =>
        finish(new DOMException('Camera stopped', 'AbortError'));
      const timer = setTimeout(() => finish(new Error(message)), timeout);
      const finish = (reason?: unknown, value?: T) => {
        clearTimeout(timer);
        lifetime.signal.removeEventListener('abort', abort);
        if (reason) reject(reason);
        else resolve(value as T);
      };
      lifetime.signal.addEventListener('abort', abort, { once: true });
      operation.then(
        (value) => finish(undefined, value),
        (reason) => finish(reason),
      );
      if (stopped) abort();
    });
  }

  async function mainDetector() {
    const { FilesetResolver, PoseLandmarker } =
      await import('@mediapipe/tasks-vision');
    const files = await FilesetResolver.forVisionTasks(asset('wasm'));
    if (stopped) return;
    const loaded = await PoseLandmarker.createFromOptions(files, {
      canvas: document.createElement('canvas'),
      baseOptions: {
        modelAssetPath: asset('pose_landmarker_full.task'),
        delegate: 'CPU',
      },
      runningMode: 'VIDEO',
      numPoses: 1,
      minPoseDetectionConfidence: 0.5,
      minPosePresenceConfidence: 0.5,
      minTrackingConfidence: 0.5,
      outputSegmentationMasks: false,
    });
    if (stopped) loaded.close();
    else detector = loaded;
  }

  async function recoverDetector() {
    if (stopped || recovering) return;
    recovering = true;
    workerUnavailable = true;
    busy = true;
    clearTimeout(frameDeadline);
    worker?.terminate();
    worker = null;
    onpose(null, performance.now());
    onstatus('กำลังปรับตัวตรวจจับให้เข้ากับกล้อง…');
    deadline = setTimeout(
      () => fail(new Error('เตรียมตัวตรวจจับท่าไม่สำเร็จ กรุณาลองใหม่')),
      20000,
    );
    try {
      await mainDetector();
      if (!stopped) {
        busy = false;
        onstatus('พร้อมแล้ว ให้ข้อมือทั้งสองอยู่ในภาพ');
      }
    } catch (reason) {
      fail(reason);
    } finally {
      recovering = false;
      clearTimeout(deadline);
    }
  }

  async function loadDetector() {
    if (
      workerUnavailable ||
      typeof OffscreenCanvas === 'undefined' ||
      typeof createImageBitmap !== 'function'
    ) {
      await mainDetector();
      return;
    }
    try {
      await new Promise<void>((resolve, reject) => {
        rejectReady = reject;
        deadline = setTimeout(
          () => reject(new Error('Pose detector timed out')),
          20000,
        );
        worker = new Worker(asset('pose-worker.js'));
        worker.onerror = () => reject(new Error('Pose worker could not load'));
        worker.onmessage = ({ data }) => {
          if (stopped) return;
          if (data.type === 'ready') resolve();
          else if (data.type === 'error') {
            if (data.unsupported) workerUnavailable = true;
            reject(new Error(data.message));
          }
        };
        worker.postMessage({
          type: 'init',
          wasm: asset('wasm'),
          model: asset('pose_landmarker_full.task'),
        });
      });
      if (worker && !stopped) {
        worker.onerror = () => void recoverDetector();
        worker.onmessage = ({ data }) => {
          if (stopped || recovering || !worker) return;
          busy = false;
          clearTimeout(frameDeadline);
          if (data.type === 'pose') onpose(data.points, data.timestamp);
          else if (data.type === 'error') void recoverDetector();
        };
      }
    } catch {
      worker?.terminate();
      worker = null;
      if (!stopped) await mainDetector();
    } finally {
      rejectReady = undefined;
      clearTimeout(deadline);
    }
  }

  function captureFrame() {
    const source = native && options.image ? options.image : video;
    const sourceWidth =
      native && options.image ? options.image.naturalWidth : video.videoWidth;
    const sourceHeight =
      native && options.image ? options.image.naturalHeight : video.videoHeight;
    const scale = Math.min(1, 640 / Math.max(sourceWidth, sourceHeight));
    const width = Math.max(1, Math.round(sourceWidth * scale));
    const height = Math.max(1, Math.round(sourceHeight * scale));
    if (input.width !== width || input.height !== height) {
      input.width = width;
      input.height = height;
    }
    inputContext ??= input.getContext('2d', {
      alpha: false,
      willReadFrequently: true,
    });
    if (!inputContext) throw new Error('เตรียมภาพสำหรับตรวจท่าไม่สำเร็จ');
    inputContext.drawImage(source, 0, 0, width, height);
    return inputContext.getImageData(0, 0, width, height);
  }

  async function frame(timestamp: number) {
    if (stopped) return;
    animation = requestAnimationFrame(frame);
    const ready = native ? native.ready : video.readyState >= 2;
    const version = native ? native.sequence : video.currentTime;
    if (
      busy ||
      document.hidden ||
      timestamp < nextFrameAt ||
      !ready ||
      version === lastFrame
    )
      return;
    busy = true;
    nextFrameAt = timestamp + 80;
    lastFrame = version;
    try {
      if (worker) {
        frameDeadline = setTimeout(() => void recoverDetector(), 5000);
        const bitmap = await createImageBitmap(captureFrame());
        if (stopped || recovering || !worker) {
          bitmap.close();
          return;
        }
        worker.postMessage({ type: 'frame', frame: bitmap, timestamp }, [
          bitmap,
        ]);
      } else if (detector) {
        inferenceTimer = setTimeout(() => {
          if (stopped || document.hidden || !detector) {
            busy = false;
            return;
          }
          const started = performance.now();
          try {
            const result = detector.detectForVideo(captureFrame(), started);
            onpose(result.landmarks[0] ?? null, started);
          } catch (reason) {
            fail(reason);
          } finally {
            busy = false;
            const finished = performance.now();
            nextFrameAt = Math.max(
              started + 100,
              finished + Math.max(32, (finished - started) / 2),
            );
          }
        }, 16);
      } else {
        busy = false;
      }
    } catch (reason) {
      if (worker) void recoverDetector();
      else fail(reason);
    }
  }

  async function start() {
    try {
      onstatus('กำลังเปิดกล้อง…');
      const backend = window.go?.main.App;
      if (backend?.CameraDevices && options.image) {
        const available = await waitFor(
          backend.CameraDevices(),
          5000,
          'อ่านรายการ webcam ไม่สำเร็จ กรุณาลองใหม่',
        );
        if (stopped) return;
        if (available.native) {
          if (available.error) throw new Error(available.error);
          options.onnative?.(true);
          const selected = preferredWebcam(available.devices, options.deviceId);
          options.ondevices?.(available.devices, selected);
          const { nativeCamera } = await import('./native-camera');
          if (stopped) return;
          native = nativeCamera(options.image, backend, fail);
          await waitFor(
            native.start(selected),
            10000,
            'เปิด webcam ไม่สำเร็จภายใน 10 วินาที กรุณาลองใหม่หรือเลือกกล้องอื่น',
          );
          if (stopped) return;
          options.onpreview?.(
            options.image.naturalWidth,
            options.image.naturalHeight,
          );
          onstatus('กำลังเตรียมตัวตรวจจับท่า…');
          await waitFor(
            loadDetector(),
            20000,
            'เตรียมตัวตรวจจับท่าไม่สำเร็จ กรุณาลองใหม่',
          );
          if (stopped) return;
          onstatus('พร้อมแล้ว ให้ข้อมือทั้งสองอยู่ในภาพ');
          animation = requestAnimationFrame(frame);
          return;
        }
      }
      options.onnative?.(false);
      if (!navigator.mediaDevices?.getUserMedia)
        throw new Error(
          'กล้องใช้ไม่ได้ในหน้าต่างนี้ กรุณาเปิด Praewa Code รุ่นล่าสุด หรือเปิดผ่าน localhost / HTTPS',
        );
      await waitFor(
        cameraAccess(true),
        5000,
        'เปิดระบบกล้องไม่สำเร็จ กรุณาลองใหม่',
      );
      if (stopped) return;
      let stream: MediaStream | null;
      try {
        stream = await waitFor(
          lease.acquire(() =>
            navigator.mediaDevices.getUserMedia(
              webcamConstraints(options.deviceId),
            ),
          ),
          10000,
          'เปิด webcam ไม่สำเร็จภายใน 10 วินาที กรุณาลองใหม่หรือเลือกกล้องอื่น',
        );
      } catch (reason) {
        if (
          !options.deviceId ||
          !(reason instanceof DOMException) ||
          !['NotFoundError', 'OverconstrainedError'].includes(reason.name)
        )
          throw reason;
        if (stopped) return;
        stream = await waitFor(
          lease.acquire(() =>
            navigator.mediaDevices.getUserMedia(webcamConstraints()),
          ),
          10000,
          'เปิด webcam ไม่สำเร็จภายใน 10 วินาที กรุณาลองใหม่หรือเลือกกล้องอื่น',
        );
      }
      if (!stream || stopped) return;
      const track = stream.getVideoTracks()[0];
      const active = track?.getSettings().deviceId || options.deviceId || '';
      const current = active
        ? [{ deviceId: active, label: track?.label || 'Webcam' }]
        : [];
      options.ondevices?.(current, active);
      stream.getVideoTracks().forEach((track) => {
        track.addEventListener(
          'ended',
          () => fail(new Error('กล้องถูกตัดการเชื่อมต่อ กรุณาลองใหม่')),
          { once: true },
        );
        track.addEventListener('mute', () => onpose(null, performance.now()));
      });
      video.srcObject = stream;
      await waitFor(
        video.play(),
        8000,
        'webcam ยังไม่ส่งภาพ กรุณาลองใหม่หรือเลือกกล้องอื่น',
      );
      if (stopped) return;
      options.onpreview?.(video.videoWidth, video.videoHeight);
      void waitFor(
        navigator.mediaDevices.enumerateDevices(),
        2500,
        'Webcam list timed out',
      )
        .then((found) => {
          if (stopped) return;
          const devices = webcamDevices(found);
          if (active && !devices.some((device) => device.deviceId === active))
            devices.unshift(...current);
          options.ondevices?.(devices, active);
        })
        .catch(() => {});
      onstatus('กำลังเตรียมตัวตรวจจับท่า…');
      await waitFor(
        loadDetector(),
        20000,
        'เตรียมตัวตรวจจับท่าไม่สำเร็จ กรุณาลองใหม่',
      );
      if (stopped) return;
      onstatus('พร้อมแล้ว ให้ข้อมือทั้งสองอยู่ในภาพ');
      animation = requestAnimationFrame(frame);
    } catch (reason) {
      fail(reason);
    }
  }

  return { start, stop };
}

export function cameraError(reason: unknown) {
  if (reason instanceof DOMException) {
    if (reason.name === 'NotAllowedError' || reason.name === 'SecurityError')
      return 'ต้องอนุญาตให้ใช้กล้องก่อน Agent จึงจะเริ่มทำงานได้';
    if (reason.name === 'NotFoundError')
      return 'ไม่พบกล้อง กรุณาเชื่อมต่อกล้องแล้วลองใหม่';
    if (reason.name === 'NotReadableError')
      return 'เปิดกล้องไม่ได้ กล้องอาจถูกใช้งานอยู่ในโปรแกรมอื่น';
  }
  return reason instanceof Error
    ? reason.message
    : 'เปิดตัวตรวจจับท่าไม่ได้ กรุณาลองใหม่';
}
