<script lang="ts">
  import { onMount } from 'svelte';
  import { MergeView } from '@codemirror/merge';
  import { EditorState } from '@codemirror/state';
  import { EditorView, lineNumbers } from '@codemirror/view';
  import type { Diff } from '../lib/types';
  let { diff }: { diff: Diff } = $props();
  let host: HTMLDivElement;
  onMount(() => {
    const theme = EditorView.theme(
      {
        '&': { height: '100%', color: '#cdd0d8', backgroundColor: '#111214' },
        '.cm-scroller': {
          fontFamily: 'JetBrains Mono, monospace',
          fontSize: '12px',
          overflow: 'auto',
        },
        '.cm-gutters': {
          backgroundColor: '#15161a',
          color: '#626874',
          border: 'none',
        },
        '.cm-content': { padding: '12px 0' },
      },
      { dark: true },
    );
    const extensions = [
      lineNumbers(),
      theme,
      EditorState.readOnly.of(true),
      EditorView.editable.of(false),
    ];
    const view = new MergeView({
      parent: host,
      a: { doc: diff.original, extensions },
      b: { doc: diff.modified, extensions },
      gutter: true,
      collapseUnchanged: { margin: 3, minSize: 8 },
    });
    return () => view.destroy();
  });
</script>

<div class="diff-labels">
  <span>Original</span><span>Modified · read only</span>
</div>
<div class="diff-host" bind:this={host}></div>

<style lang="postcss">
  @reference '../styles.css';

  .diff-labels {
    @apply flex py-[10px] px-[16px] bg-[#17181b] text-[11px] text-[#9ca3af];
  }
  .diff-labels span {
    @apply w-1/2;
  }
  .diff-host {
    @apply h-[calc(100%_-_34px)] overflow-auto;
  }
  .diff-host :global(.cm-mergeView) {
    @apply h-full;
  }
  .diff-host :global(.cm-mergePane) {
    @apply h-full;
  }
  .diff-host :global(.cm-merge-a .cm-changedLine) {
    @apply bg-[#ec888118];
  }
  .diff-host :global(.cm-merge-b .cm-changedLine) {
    @apply bg-[#a5d6bd18];
  }
</style>
