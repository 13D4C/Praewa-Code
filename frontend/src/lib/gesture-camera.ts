import type { PoseLandmarker } from '@mediapipe/tasks-vision';
import type { PosePoint } from './six-seven';

let accessQueue = Promise.resolve();

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
  } = {},
) {
  const lease = new CameraLease();
  let stopped = false;
  let worker: Worker | null = null;
  let detector: PoseLandmarker | null = null;
  let animation = 0;
  let deadline: ReturnType<typeof setTimeout> | undefined;
  let frameDeadline: ReturnType<typeof setTimeout> | undefined;
  let busy = false;
  let recovering = false;
  let lastFrame = -1;
  let lastTime = -Infinity;
  let rejectReady: ((reason: unknown) => void) | undefined;
  const asset = (path: string) =>
    new URL(`/vision/${path}`, window.location.href).href;

  function stop() {
    if (stopped) return;
    stopped = true;
    clearTimeout(deadline);
    clearTimeout(frameDeadline);
    cancelAnimationFrame(animation);
    lease.stop();
    video.srcObject = null;
    rejectReady?.(new Error('Camera stopped'));
    worker?.terminate();
    worker = null;
    detector?.close();
    detector = null;
    void cameraAccess(false).catch(() => {});
  }

  function fail(reason: unknown) {
    if (stopped) return;
    stop();
    onerror(reason);
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
          else if (data.type === 'error') reject(new Error(data.message));
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

  async function frame(timestamp: number) {
    if (stopped) return;
    animation = requestAnimationFrame(frame);
    if (
      busy ||
      timestamp - lastTime < 50 ||
      video.readyState < 2 ||
      video.currentTime === lastFrame
    )
      return;
    busy = true;
    lastTime = timestamp;
    lastFrame = video.currentTime;
    try {
      if (worker) {
        frameDeadline = setTimeout(() => void recoverDetector(), 5000);
        const bitmap = await createImageBitmap(video);
        if (stopped || recovering || !worker) {
          bitmap.close();
          return;
        }
        worker.postMessage({ type: 'frame', frame: bitmap, timestamp }, [
          bitmap,
        ]);
      } else if (detector) {
        const result = detector.detectForVideo(video, timestamp);
        busy = false;
        onpose(result.landmarks[0] ?? null, timestamp);
      }
    } catch (reason) {
      if (worker) void recoverDetector();
      else fail(reason);
    }
  }

  async function start() {
    try {
      if (!navigator.mediaDevices?.getUserMedia)
        throw new Error(
          'กล้องใช้ไม่ได้ในหน้าต่างนี้ กรุณาเปิด Praewa Code รุ่นล่าสุด หรือเปิดผ่าน localhost / HTTPS',
        );
      onstatus('กำลังเปิดกล้อง…');
      await cameraAccess(true);
      if (stopped) return;
      let stream: MediaStream | null;
      try {
        stream = await lease.acquire(() =>
          navigator.mediaDevices.getUserMedia(
            webcamConstraints(options.deviceId),
          ),
        );
      } catch (reason) {
        if (
          !options.deviceId ||
          !(reason instanceof DOMException) ||
          !['NotFoundError', 'OverconstrainedError'].includes(reason.name)
        )
          throw reason;
        if (stopped) return;
        stream = await lease.acquire(() =>
          navigator.mediaDevices.getUserMedia(webcamConstraints()),
        );
      }
      if (!stream || stopped) return;
      const devices = webcamDevices(
        await navigator.mediaDevices.enumerateDevices(),
      );
      if (stopped) return;
      const selected = preferredWebcam(devices, options.deviceId);
      if (
        selected &&
        stream.getVideoTracks()[0]?.getSettings().deviceId !== selected
      ) {
        stream = await lease.acquire(() =>
          navigator.mediaDevices.getUserMedia(webcamConstraints(selected)),
        );
      }
      if (!stream || stopped) return;
      const active =
        stream.getVideoTracks()[0]?.getSettings().deviceId || selected;
      options.ondevices?.(devices, active);
      stream.getVideoTracks().forEach((track) => {
        track.addEventListener(
          'ended',
          () => fail(new Error('กล้องถูกตัดการเชื่อมต่อ กรุณาลองใหม่')),
          { once: true },
        );
        track.addEventListener('mute', () => onpose(null, performance.now()));
      });
      video.srcObject = stream;
      await video.play();
      if (stopped) return;
      options.onpreview?.(video.videoWidth, video.videoHeight);
      onstatus('กำลังเตรียมตัวตรวจจับท่า…');
      await loadDetector();
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
