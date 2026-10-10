importScripts('vision.js');

let detector;
let loading = false;

self.onmessage = async ({ data }) => {
  try {
    if (data.type === 'init' && !loading) {
      loading = true;
      const canvas = new OffscreenCanvas(640, 480);
      if (!canvas.getContext('webgl2')) {
        self.postMessage({
          type: 'error',
          unsupported: true,
          message: 'Worker WebGL is unavailable',
        });
        return;
      }
      const files = await PraewaVision.FilesetResolver.forVisionTasks(
        data.wasm,
      );
      detector = await PraewaVision.PoseLandmarker.createFromOptions(files, {
        canvas,
        baseOptions: { modelAssetPath: data.model, delegate: 'CPU' },
        runningMode: 'VIDEO',
        numPoses: 1,
        minPoseDetectionConfidence: 0.5,
        minPosePresenceConfidence: 0.5,
        minTrackingConfidence: 0.5,
        outputSegmentationMasks: false,
      });
      self.postMessage({ type: 'ready' });
    } else if (data.type === 'frame' && detector) {
      try {
        const result = detector.detectForVideo(data.frame, data.timestamp);
        self.postMessage({
          type: 'pose',
          points: result.landmarks[0] ?? null,
          timestamp: data.timestamp,
        });
      } finally {
        data.frame.close();
      }
    }
  } catch (reason) {
    self.postMessage({ type: 'error', message: String(reason) });
  }
};
