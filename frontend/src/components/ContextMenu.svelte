<script lang="ts">
  import { tick } from 'svelte';
  import type { MenuItem } from './MenuBar.svelte';
  let {
    x,
    y,
    items,
    onclose,
    onerror,
  }: {
    x: number;
    y: number;
    items: MenuItem[];
    onclose: () => void;
    onerror: (error: unknown) => void;
  } = $props();
  let menu: HTMLDivElement;
  let left = $state(0);
  let top = $state(0);
  function choices() {
    return Array.from(
      menu.querySelectorAll<HTMLButtonElement>('button:not(:disabled)'),
    );
  }
  $effect(() => {
    x;
    y;
    void tick().then(() => {
      left = Math.max(8, Math.min(x, window.innerWidth - menu.offsetWidth - 8));
      top = Math.max(
        8,
        Math.min(y, window.innerHeight - menu.offsetHeight - 8),
      );
      choices()[0]?.focus();
    });
  });
  function outside(event: Event) {
    if (event.target instanceof Node && !menu.contains(event.target)) onclose();
  }
  function keydown(event: KeyboardEvent) {
    if (event.key === 'Escape' || event.key === 'Tab') {
      if (event.key === 'Escape') event.preventDefault();
      onclose();
      return;
    }
    const buttons = choices();
    const index = buttons.indexOf(document.activeElement as HTMLButtonElement);
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault();
      buttons[
        (index + (event.key === 'ArrowDown' ? 1 : buttons.length - 1)) %
          buttons.length
      ]?.focus();
    } else if (event.key === 'Home' || event.key === 'End') {
      event.preventDefault();
      (event.key === 'Home' ? buttons[0] : buttons.at(-1))?.focus();
    }
  }
  async function select(item: MenuItem) {
    onclose();
    try {
      await item.action();
    } catch (error) {
      onerror(error);
    }
  }
</script>

<svelte:window
  onpointerdown={outside}
  onfocusin={outside}
  onblur={onclose}
  onresize={onclose}
/>
<div
  class="file-context-menu"
  role="menu"
  aria-label="File actions"
  tabindex="-1"
  bind:this={menu}
  style:left={`${left}px`}
  style:top={`${top}px`}
  onkeydown={keydown}
>
  {#each items as item}<button
      role="menuitem"
      disabled={item.disabled}
      tabindex="-1"
      onclick={() => void select(item)}
      ><item.icon size={14} /><span>{item.label}</span>{#if item.key}<kbd
          >{item.key}</kbd
        >{/if}</button
    >{/each}
</div>

<style lang="postcss">
  @reference '../styles.css';

  .file-context-menu {
    @apply fixed z-[40] min-w-[210px] max-w-[calc(100vw_-_16px)] max-h-[calc(100dvh_-_16px)] overflow-y-auto p-[5px] bg-[#18191d] border border-[#3c3b30] border-t-2 border-t-accent rounded-[6px];
    box-shadow: 0 10px 28px #0008;
  }
  button {
    @apply flex w-full gap-[9px] items-center p-[8px] text-[11px] rounded-[3px] text-left;
  }
  button:hover:not(:disabled),
  button:focus-visible {
    @apply bg-[#f5d54716] text-accent;
    outline: none;
  }
  button span {
    @apply flex-1;
  }
  kbd {
    @apply ml-[20px] text-[#898e99];
  }
</style>
