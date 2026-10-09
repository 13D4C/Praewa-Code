<script lang="ts">
  import { onMount } from 'svelte';
  import { Camera, LockKeyhole, RefreshCw, X } from '@lucide/svelte';
  import {
    SixSevenCounter,
    SIX_SEVEN_TARGET,
    type TrackedArm,
  } from '../lib/six-seven';
  import {
    gestureCamera,
    cameraError,
    type WebcamDevice,
  } from '../lib/gesture-camera';

  let {
    oncomplete,
    oncancel,
  }: {
    oncomplete: (count: number) => void;
    oncancel: () => void;
  } = $props();
  let video: HTMLVideoElement;
  let count = $state(0);
  let tracking = $state(false);
  let halfway = $state(false);
  let ready = $state(false);
  let previewReady = $state(false);
  let status = $state('กำลังเปิดกล้อง…');
  let error = $state('');
  let arms = $state<TrackedArm[]>([]);
  let direction = $state(0);
  let webcams = $state<WebcamDevice[]>([]);
  let selectedWebcam = $state('');
  let aspectRatio = $state(4 / 3);
  let session: ReturnType<typeof gestureCamera> | null = null;
  let disposed = false;
  let attempt = 0;

  function start() {
    session?.stop();
    const token = ++attempt;
    const counter = new SixSevenCounter();
    count = 0;
    error = '';
    ready = false;
    previewReady = false;
    tracking = false;
    halfway = false;
    arms = [];
    direction = 0;
    session = gestureCamera(
      video,
      (points, timestamp) => {
        if (disposed || token !== attempt || document.hidden) return;
        ready = true;
        const progress = counter.update(points, timestamp, aspectRatio);
        count = progress.count;
        tracking = progress.tracking;
        halfway = progress.halfway;
        arms = progress.arms;
        direction = progress.direction;
        if (count === SIX_SEVEN_TARGET) {
          ++attempt;
          session?.stop();
          oncomplete(count);
        }
      },
      (message) => {
        if (!disposed && token === attempt) status = message;
      },
      (reason) => {
        if (!disposed && token === attempt) {
          error = cameraError(reason);
          ready = false;
          previewReady = false;
          tracking = false;
          arms = [];
        }
      },
      {
        deviceId: selectedWebcam,
        ondevices: (devices, selected) => {
          if (disposed || token !== attempt) return;
          webcams = devices;
          selectedWebcam = selected;
          try {
            localStorage.setItem('praewa-six-seven-webcam', selected);
          } catch {}
        },
        onpreview: (width, height) => {
          if (!disposed && token === attempt && width && height) {
            aspectRatio = width / height;
            previewReady = true;
          }
        },
      },
    );
    void session.start();
  }

  function cancel() {
    ++attempt;
    session?.stop();
    oncancel();
  }

  onMount(() => {
    try {
      selectedWebcam = localStorage.getItem('praewa-six-seven-webcam') ?? '';
    } catch {}
    const hide = () => {
      if (document.hidden) cancel();
    };
    document.addEventListener('visibilitychange', hide);
    window.addEventListener('pagehide', cancel);
    start();
    return () => {
      disposed = true;
      ++attempt;
      session?.stop();
      document.removeEventListener('visibilitychange', hide);
      window.removeEventListener('pagehide', cancel);
    };
  });
</script>

<section
  class="mx-3 my-2 shrink-0 overflow-hidden rounded-xl border border-accent/35 bg-canvas"
  aria-label="Six Seven challenge"
>
  <div class="flex items-center justify-between gap-2 px-3 py-2">
    <div class="flex items-center gap-2 text-accent">
      <LockKeyhole size={14} /><span
        class="text-[11px] font-bold tracking-widest">SIX SEVEN TO UNLOCK</span
      >
    </div>
    <button
      class="icon-button"
      aria-label="Cancel Six Seven challenge"
      title="ยกเลิก"
      onclick={cancel}><X size={14} /></button
    >
  </div>
  <div
    class="relative mx-auto w-[calc(100%_-_16px)] max-w-[350px] overflow-hidden rounded-lg bg-black"
    style:aspect-ratio={aspectRatio}
  >
    <video
      bind:this={video}
      autoplay
      muted
      playsinline
      class="h-full w-full -scale-x-100 object-cover"
      aria-label="ภาพกล้องสำหรับนับท่า Six Seven"
    ></video>
    <svg
      class="pointer-events-none absolute inset-0 h-full w-full"
      viewBox="0 0 100 100"
      preserveAspectRatio="none"
      aria-hidden="true"
    >
      {#each arms as arm}
        <g
          stroke={arm.side === 'left' ? '#facc15' : '#e2e8f0'}
          fill={arm.side === 'left' ? '#facc15' : '#e2e8f0'}
          opacity={tracking ? 0.95 : 0.5}
        >
          {#if arm.shoulder && arm.elbow}
            <line
              x1={(1 - arm.shoulder.x) * 100}
              y1={arm.shoulder.y * 100}
              x2={(1 - arm.elbow.x) * 100}
              y2={arm.elbow.y * 100}
              stroke-width="3"
              vector-effect="non-scaling-stroke"
            />
          {/if}
          {#if arm.elbow}
            <line
              x1={(1 - arm.elbow.x) * 100}
              y1={arm.elbow.y * 100}
              x2={(1 - arm.wrist.x) * 100}
              y2={arm.wrist.y * 100}
              stroke-width="3"
              vector-effect="non-scaling-stroke"
            />
          {/if}
          {#each [arm.shoulder, arm.elbow, arm.wrist] as point}
            {#if point}
              <circle
                cx={(1 - point.x) * 100}
                cy={point.y * 100}
                r="1.1"
                stroke="none"
              />
            {/if}
          {/each}
        </g>
      {/each}
    </svg>
    {#each arms as arm}
      <div
        class="pointer-events-none absolute flex h-7 -translate-x-1/2 -translate-y-1/2 items-center gap-1 rounded-full border border-accent bg-black/75 px-2 text-xs font-bold text-accent"
        style:left={`${(1 - arm.wrist.x) * 100}%`}
        style:top={`${arm.wrist.y * 100}%`}
      >
        {arm.side === 'left' ? '6' : '7'}
        {#if tracking && direction}
          <span
            >{(arm.side === 'left' ? direction : -direction) > 0
              ? '↑'
              : '↓'}</span
          >
        {/if}
      </div>
    {/each}
    {#if !ready && !error}
      {#if previewReady}
        <div
          class="pointer-events-none absolute left-2 right-2 top-2 rounded-md bg-black/65 px-2 py-1.5 text-center text-[11px] text-foreground"
        >
          {status}
        </div>
      {:else}
        <div
          class="absolute inset-0 flex flex-col items-center justify-center gap-2 bg-black/75 px-4 text-center text-xs text-foreground"
        >
          <Camera size={26} class="text-accent" />{status}
        </div>
      {/if}
    {/if}
    <div
      class="absolute bottom-2 right-2 rounded-lg bg-black/80 px-3 py-1 font-mono text-3xl font-bold text-accent"
      aria-label={`${count} of ${SIX_SEVEN_TARGET} repetitions`}
    >
      {count}<span class="ml-1 text-sm text-muted">/ {SIX_SEVEN_TARGET}</span>
    </div>
  </div>
  <div class="space-y-2 px-3 pb-3 pt-2">
    {#if webcams.length}
      <label class="flex items-center gap-2 text-[11px] text-muted">
        <Camera size={13} /><span>Webcam</span>
        <select
          class="min-w-0 flex-1 rounded-md border border-line bg-surface px-2 py-1.5 text-[11px] text-foreground"
          aria-label="เลือกเว็บแคม"
          value={selectedWebcam}
          onchange={(event) => {
            selectedWebcam = event.currentTarget.value;
            start();
          }}
        >
          {#each webcams as webcam, index}
            <option value={webcam.deviceId}
              >{webcam.label}{webcams.filter(
                (item) => item.label === webcam.label,
              ).length > 1
                ? ` · ${index + 1}`
                : ''}</option
            >
          {/each}
        </select>
      </label>
    {/if}
    {#if ready && !error}
      <div class="flex items-center justify-between text-[11px]">
        <span class={tracking ? 'text-accent' : 'text-muted'}
          >พบข้อมือ {arms.length}/2</span
        >
        <span class="text-muted"
          >{halfway ? 'ครึ่งรอบ · สลับกลับ' : 'ขึ้น–ลงสวนกัน'}</span
        >
      </div>
    {/if}
    <div
      class="h-1.5 overflow-hidden rounded-full bg-white/10"
      role="progressbar"
      aria-label="Six Seven progress"
      aria-valuenow={count}
      aria-valuemin={0}
      aria-valuemax={SIX_SEVEN_TARGET}
    >
      <div
        class="h-full bg-accent transition-[width] duration-150"
        style:width={`${(count / SIX_SEVEN_TARGET) * 100}%`}
      ></div>
    </div>
    {#if error}
      <p class="text-xs text-red-300" role="alert">{error}</p>
      <button
        class="flex w-full items-center justify-center gap-2 rounded-md border border-accent/40 py-2 text-xs text-accent hover:bg-accent/10"
        onclick={start}><RefreshCw size={13} />ลองเปิดกล้องอีกครั้ง</button
      >
    {:else}
      <p class="m-0 text-xs text-foreground" role="status">
        {!ready
          ? 'Agent กำลังรอให้ผ่านด่าน'
          : !tracking
            ? 'ให้ข้อมือซ้ายและขวาอยู่ในภาพ'
            : halfway
              ? 'สลับข้อมือขึ้น–ลงกลับอีกฝั่งให้ครบหนึ่งรอบ'
              : 'ยกข้อมือหนึ่งขึ้น พร้อมลดอีกข้างลง · Six… Seven…'}
      </p>
    {/if}
    <p class="m-0 text-[11px] leading-relaxed text-muted">
      สลับซ้าย–ขวาแล้วกลับ = 1 ครั้ง · ครบ 50 ส่งข้อความอัตโนมัติ<br
      />ภาพกล้องประมวลผลในเครื่อง และปิดเมื่อจบหรือยกเลิก
    </p>
  </div>
</section>
