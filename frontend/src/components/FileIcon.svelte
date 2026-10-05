<script lang="ts">
  import { FileText } from '@lucide/svelte';
  import { fileIcon } from '../lib/file-icons';

  let { name, size = 16 }: { name: string; size?: number } = $props();
  const source = $derived(fileIcon(name));
</script>

{#if source}
  <img
    class="file-type-icon block flex-none object-contain"
    src={source}
    alt=""
    aria-hidden="true"
    draggable="false"
    width={size}
    height={size}
  />
{:else}
  <FileText {size} class="muted" aria-hidden="true" />
{/if}
