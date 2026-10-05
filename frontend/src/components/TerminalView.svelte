<script lang="ts">
  import { onMount, tick } from 'svelte';
  import { Terminal } from '@xterm/xterm';
  import { FitAddon } from '@xterm/addon-fit';
  import { WebLinksAddon } from '@xterm/addon-web-links';
  import '@xterm/xterm/css/xterm.css';
  import { backend, onEvent, openLink } from '../lib/bridge';
  import type { TerminalEvent } from '../lib/types';
  let {
    id,
    visible,
    fontSize,
    onexit,
    onerror,
  }: {
    id: string;
    visible: boolean;
    fontSize: number;
    onexit: (code: number) => void;
    onerror: (error: unknown) => void;
  } = $props();
  let host: HTMLDivElement;
  let terminal: Terminal | undefined;
  let fit: FitAddon;
  let mounted = $state(false);
  let lastSequence = 0;
  function resize() {
    if (!visible || !terminal || !host.clientWidth || !host.clientHeight)
      return;
    fit.fit();
    void backend()
      .TerminalResize(id, terminal.cols, terminal.rows)
      .catch(() => {});
  }
  function receive(event: TerminalEvent) {
    if (event.sequence <= lastSequence) return;
    lastSequence = event.sequence;
    if (event.data)
      terminal?.write(
        Uint8Array.from(atob(event.data), (char) => char.charCodeAt(0)),
      );
    if (event.exited) {
      terminal?.writeln(
        `\r\n\x1b[33mProcess exited with code ${event.exitCode}.\x1b[0m`,
      );
      onexit(event.exitCode);
    }
  }
  onMount(() => {
    const instance = new Terminal({
      cursorBlink: true,
      fontFamily: 'JetBrains Mono, monospace',
      fontSize,
      scrollback: 10000,
      allowProposedApi: false,
      theme: {
        background: '#101113',
        foreground: '#cdd0d8',
        cursor: '#f5d547',
        selectionBackground: '#f5d54735',
        black: '#262931',
        red: '#ed8881',
        green: '#a5d6bd',
        yellow: '#f5d547',
        blue: '#82c5e7',
        magenta: '#d6b6fb',
        cyan: '#8fd6d6',
        white: '#d9dbe1',
        brightBlack: '#6b7280',
      },
    });
    terminal = instance;
    fit = new FitAddon();
    instance.loadAddon(fit);
    instance.loadAddon(
      new WebLinksAddon((event, url) => {
        event.preventDefault();
        openLink(url);
      }),
    );
    instance.open(host);
    let ready = false;
    let disposed = false;
    const pending: TerminalEvent[] = [];
    const unsubscribe = onEvent<TerminalEvent>('terminal:event', (event) => {
      if (event.id !== id) return;
      if (ready) receive(event);
      else pending.push(event);
    });
    void backend()
      .TerminalSnapshot(id)
      .then((snapshot) => {
        if (disposed) return;
        for (const chunk of snapshot.chunks) receive(chunk);
        lastSequence = snapshot.sequence;
        ready = true;
        for (const event of pending.sort((a, b) => a.sequence - b.sequence))
          receive(event);
        if (snapshot.info.exited) {
          instance.writeln(
            `\r\nProcess exited with code ${snapshot.info.exitCode}.`,
          );
          onexit(snapshot.info.exitCode);
        }
        mounted = true;
        resize();
      })
      .catch(onerror);
    const input = instance.onData((data) => {
      void backend().TerminalWrite(id, data).catch(onerror);
    });
    const observer = new ResizeObserver(() => resize());
    observer.observe(host);
    return () => {
      disposed = true;
      unsubscribe();
      observer.disconnect();
      input.dispose();
      instance.dispose();
      terminal = undefined;
    };
  });
  $effect(() => {
    const shown = visible;
    const size = fontSize;
    if (mounted && terminal) {
      terminal.options.fontSize = size;
      if (shown)
        void tick().then(() => {
          resize();
          terminal?.focus();
        });
    }
  });
</script>

<div class="terminal-view" class:hidden={!visible} bind:this={host}></div>

<style lang="postcss">
  @reference '../styles.css';

  .terminal-view {
    @apply h-full min-h-0 pt-[8px] pr-[12px] pb-[4px] pl-[12px];
  }
  .hidden {
    @apply hidden;
  }
  .terminal-view :global(.xterm) {
    @apply h-full;
  }
  .terminal-view :global(.xterm-viewport) {
    scrollbar-width: thin;
  }
</style>
