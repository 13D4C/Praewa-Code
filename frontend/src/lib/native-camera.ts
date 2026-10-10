import type { WebcamDevice } from './gesture-camera';

export type NativeCameraDevices = {
  native: boolean;
  devices: WebcamDevice[];
  error: string;
};
export type NativeCameraFrame = {
  sequence: number;
  image: string;
  error: string;
};
export type NativeCameraBackend = {
  CameraDevices(): Promise<NativeCameraDevices>;
  CameraOpen(id: string, device: string): Promise<string>;
  CameraFrame(id: string, after: number): Promise<NativeCameraFrame>;
  CameraClose(id: string): Promise<void>;
};

export function nativeCamera(
  image: HTMLImageElement,
  backend: NativeCameraBackend,
  onerror: (reason: unknown) => void,
) {
  const id = crypto.randomUUID();
  let stopped = false;
  let polling = false;
  let timer: ReturnType<typeof setInterval> | undefined;
  let sequence = 0;
  let resolvePreview: () => void;
  let rejectPreview: (reason: unknown) => void;
  const preview = new Promise<void>((resolve, reject) => {
    resolvePreview = resolve;
    rejectPreview = reject;
  });
  void preview.catch(() => {});

  function stop() {
    if (stopped) return;
    stopped = true;
    clearInterval(timer);
    image.removeAttribute('src');
    rejectPreview(new DOMException('Camera stopped', 'AbortError'));
    void backend.CameraClose(id).catch(() => {});
  }
  async function poll() {
    if (stopped || polling || document.hidden) return;
    polling = true;
    try {
      const frame = await backend.CameraFrame(id, sequence);
      if (stopped) return;
      if (frame.error) throw new Error(frame.error);
      if (!frame.image || frame.sequence <= sequence) return;
      image.src = frame.image;
      await image.decode();
      if (stopped) return;
      sequence = frame.sequence;
      resolvePreview();
    } catch (reason) {
      if (!stopped) {
        rejectPreview(reason);
        stop();
        onerror(reason);
      }
    } finally {
      polling = false;
    }
  }
  return {
    async start(device: string) {
      const active = await backend.CameraOpen(id, device);
      if (stopped) return active;
      timer = setInterval(() => void poll(), 80);
      void poll();
      await preview;
      return active;
    },
    stop,
    get sequence() {
      return sequence;
    },
    get ready() {
      return sequence > 0 && image.complete && image.naturalWidth > 0;
    },
  };
}
