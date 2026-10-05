<script lang="ts">
  import { marked } from 'marked';
  import DOMPurify from 'dompurify';
  import { openLink } from '../lib/bridge';
  let { content }: { content: string } = $props();
  const html = $derived(
    DOMPurify.sanitize(marked.parse(content, { async: false }), {
      FORBID_TAGS: ['iframe', 'style'],
      FORBID_ATTR: ['style'],
    }),
  );
  function handleLinks(node: HTMLDivElement) {
    const handler = (event: MouseEvent) => {
      const link = (event.target as HTMLElement).closest('a');
      if (link) {
        event.preventDefault();
        openLink(link.href);
      }
    };
    node.addEventListener('click', handler);
    return {
      destroy() {
        node.removeEventListener('click', handler);
      },
    };
  }
</script>

<div class="markdown-preview" role="document" use:handleLinks>{@html html}</div>

<style lang="postcss">
  @reference '../styles.css';

  .markdown-preview {
    @apply h-full overflow-auto py-[24px] px-[34px] text-[#cdd0d8] text-[14px] leading-[1.8];
  }
  .markdown-preview :global(h1),
  .markdown-preview :global(h2) {
    @apply border-b border-b-[#353840] pb-[8px] text-[#e8e9ec];
  }
  .markdown-preview :global(a) {
    @apply text-accent;
  }
  .markdown-preview :global(pre) {
    @apply bg-[#1c1d22] p-[18px] rounded-[6px] overflow-auto;
  }
  .markdown-preview :global(code) {
    @apply font-mono text-[12px];
  }
  .markdown-preview :global(img) {
    @apply max-w-full;
  }
  .markdown-preview :global(blockquote) {
    @apply border-l-3 border-l-accent pl-[18px] text-[#939aa5];
  }
  .markdown-preview :global(table) {
    @apply border-collapse;
  }
  .markdown-preview :global(td),
  .markdown-preview :global(th) {
    @apply border border-[#353840] py-[8px] px-[12px];
  }
</style>
