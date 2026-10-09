export const SIX_SEVEN_TARGET = 50;

export type PosePoint = {
  x: number;
  y: number;
  visibility?: number;
  presence?: number;
};
export type TrackedArm = {
  side: 'left' | 'right';
  shoulder: PosePoint | null;
  elbow: PosePoint | null;
  wrist: PosePoint;
  size: number;
};
export type RepProgress = {
  count: number;
  tracking: boolean;
  halfway: boolean;
  arms: TrackedArm[];
  issue: 'missing' | 'ready';
  direction: number;
};

type Position = {
  left: number;
  right: number;
  leftSize: number;
  rightSize: number;
};

export class SixSevenCounter {
  private count = 0;
  private anchor: Position | null = null;
  private smooth: Position | null = null;
  private lastPosition: Position | null = null;
  private direction = 0;
  private halfway = false;
  private candidate = 0;
  private candidateSince = 0;
  private lastTime = -1;
  private lastSeen = -Infinity;
  private lastTransition = -Infinity;

  reset() {
    this.count = 0;
    this.lastTime = -1;
    this.lastSeen = -Infinity;
    this.loseTracking();
  }

  private loseTracking() {
    this.anchor = null;
    this.smooth = null;
    this.lastPosition = null;
    this.direction = 0;
    this.halfway = false;
    this.candidate = 0;
    this.lastTransition = -Infinity;
  }

  update(
    points: PosePoint[] | null,
    timestamp: number,
    aspectRatio = 4 / 3,
  ): RepProgress {
    if (!Number.isFinite(timestamp) || timestamp <= this.lastTime)
      return this.progress([], 'missing');
    const elapsed = this.lastTime < 0 ? 50 : timestamp - this.lastTime;
    this.lastTime = timestamp;
    const aspect =
      Number.isFinite(aspectRatio) && aspectRatio > 0 ? aspectRatio : 4 / 3;
    const arms = [
      this.arm(points, 'left', aspect),
      this.arm(points, 'right', aspect),
    ].filter((arm): arm is TrackedArm => arm !== null);
    if (arms.length !== 2) {
      this.candidate = 0;
      if (timestamp - this.lastSeen > 300) this.loseTracking();
      return this.progress(arms, 'missing');
    }
    if (timestamp - this.lastSeen > 300) this.loseTracking();
    this.lastSeen = timestamp;
    const position = {
      left: arms[0].wrist.y,
      right: arms[1].wrist.y,
      leftSize: arms[0].size,
      rightSize: arms[1].size,
    };
    const previousPosition = this.lastPosition ?? position;
    this.lastPosition = position;
    if (!this.smooth) this.smooth = { ...position };
    else {
      const alpha = 1 - Math.exp(-Math.min(elapsed, 150) / 45);
      this.smooth.left += alpha * (position.left - this.smooth.left);
      this.smooth.right += alpha * (position.right - this.smooth.right);
      this.smooth.leftSize += 0.15 * (position.leftSize - this.smooth.leftSize);
      this.smooth.rightSize +=
        0.15 * (position.rightSize - this.smooth.rightSize);
    }
    const current = this.smooth;
    if (!this.anchor) {
      this.anchor = { ...current };
      return this.progress(arms, 'ready');
    }
    const leftThreshold = Math.max(
      0.022,
      Math.min(0.06, current.leftSize * 0.12),
    );
    const rightThreshold = Math.max(
      0.022,
      Math.min(0.06, current.rightSize * 0.12),
    );
    const leftTravel = current.left - this.anchor.left;
    const rightTravel = current.right - this.anchor.right;
    const side =
      leftTravel >= leftThreshold && rightTravel <= -rightThreshold
        ? 1
        : leftTravel <= -leftThreshold && rightTravel >= rightThreshold
          ? -1
          : 0;
    if (this.direction) {
      const leftPeak =
        this.direction > 0
          ? Math.min(current.left, position.left, previousPosition.left)
          : Math.max(current.left, position.left, previousPosition.left);
      const rightPeak =
        this.direction > 0
          ? Math.max(current.right, position.right, previousPosition.right)
          : Math.min(current.right, position.right, previousPosition.right);
      if ((leftPeak - this.anchor.left) * this.direction > 0)
        this.anchor.left = leftPeak;
      if ((rightPeak - this.anchor.right) * this.direction < 0)
        this.anchor.right = rightPeak;
    }
    if (
      !side ||
      side === this.direction ||
      (position.left - this.anchor.left) * side < leftThreshold * 0.8 ||
      (position.right - this.anchor.right) * side > -rightThreshold * 0.8
    ) {
      this.candidate = 0;
      return this.progress(arms, 'ready');
    }
    if (this.candidate !== side) {
      this.candidate = side;
      this.candidateSince = timestamp;
      return this.progress(arms, 'ready');
    }
    if (
      timestamp - this.candidateSince < 45 ||
      timestamp - this.lastTransition < 120
    )
      return this.progress(arms, 'ready');
    this.anchor = { ...current };
    this.direction = side;
    this.candidate = 0;
    this.lastTransition = timestamp;
    if (this.halfway) {
      this.count = Math.min(SIX_SEVEN_TARGET, this.count + 1);
      this.halfway = false;
    } else this.halfway = true;
    return this.progress(arms, 'ready');
  }

  private progress(
    arms: TrackedArm[],
    issue: RepProgress['issue'],
  ): RepProgress {
    return {
      count: this.count,
      tracking: issue === 'ready',
      halfway: this.halfway,
      arms,
      issue,
      direction: this.direction,
    };
  }

  private arm(
    points: PosePoint[] | null,
    side: TrackedArm['side'],
    aspect: number,
  ): TrackedArm | null {
    const indices = side === 'left' ? [11, 13, 15] : [12, 14, 16];
    const valid = (point: PosePoint | undefined, wrist = false) => {
      if (!point || !Number.isFinite(point.x) || !Number.isFinite(point.y))
        return false;
      if ((point.visibility ?? 0) < 0.35 || (point.presence ?? 1) < 0.35)
        return false;
      const margin = wrist ? 0 : -0.2;
      return (
        point.x >= margin &&
        point.x <= 1 - margin &&
        point.y >= margin &&
        point.y <= 1 - margin
      );
    };
    const wrist = points?.[indices[2]];
    if (!valid(wrist, true) || !wrist) return null;
    const shoulder = valid(points?.[indices[0]]) ? points![indices[0]] : null;
    const elbow = valid(points?.[indices[1]]) ? points![indices[1]] : null;
    const distance = (a: PosePoint, b: PosePoint) =>
      Math.hypot((a.x - b.x) * aspect, a.y - b.y);
    const size =
      shoulder && elbow
        ? distance(shoulder, elbow)
        : elbow
          ? distance(elbow, wrist)
          : 0.25;
    return { side, shoulder, elbow, wrist, size };
  }
}

export class PromptChallenge<T> {
  private pending: { id: string; value: T } | null = null;

  begin(id: string, value: T) {
    this.pending = { id, value };
  }

  cancel() {
    this.pending = null;
  }

  complete(id: string, count: number): T | null {
    if (!this.pending || this.pending.id !== id || count !== SIX_SEVEN_TARGET)
      return null;
    const value = this.pending.value;
    this.pending = null;
    return value;
  }
}
