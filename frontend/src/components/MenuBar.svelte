<script module lang="ts">
  import type { LucideIcon } from '@lucide/svelte';

  export type MenuItem = {
    label: string;
    key: string;
    icon: LucideIcon;
    action: () => unknown;
    disabled?: boolean;
    checked?: boolean;
  };

  export type MenuDefinition = {
    label: string;
    groups: MenuItem[][];
  };
</script>

<script lang="ts">
  import { tick } from 'svelte';
  import { Check } from '@lucide/svelte';

  let {
    menus,
    onerror,
  }: {
    menus: MenuDefinition[];
    onerror: (error: unknown) => void;
  } = $props();
  const id = $props.id();
  let root: HTMLElement;
  let opened = $state<number | null>(null);
  let openedByHover = false;
  let focusedMenu = $state(0);
  const triggers: HTMLButtonElement[] = [];

  function items() {
    return Array.from(
      root.querySelectorAll<HTMLButtonElement>(
        '.dropdown-menu button:not(:disabled)',
      ),
    );
  }

  async function openMenu(
    index: number,
    edge: 'first' | 'last' = 'first',
    hover = false,
  ) {
    opened = index;
    openedByHover = hover;
    focusedMenu = index;
    await tick();
    if (opened !== index) return;
    const choices = items();
    (edge === 'last' ? choices.at(-1) : choices[0])?.focus();
  }

  function closeMenu(restoreFocus = false) {
    const index = opened;
    opened = null;
    if (restoreFocus && index !== null) triggers[index]?.focus();
  }

  function outside(event: Event) {
    if (
      opened !== null &&
      event.target instanceof Node &&
      !root.contains(event.target)
    )
      closeMenu();
  }

  function triggerKeydown(event: KeyboardEvent, index: number) {
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault();
      void openMenu(index, event.key === 'ArrowUp' ? 'last' : 'first');
    } else if (event.key === 'ArrowRight' || event.key === 'ArrowLeft') {
      event.preventDefault();
      focusedMenu =
        (index + (event.key === 'ArrowRight' ? 1 : menus.length - 1)) %
        menus.length;
      if (opened !== null) void openMenu(focusedMenu);
      else triggers[focusedMenu]?.focus();
    } else if (event.key === 'Escape') {
      event.preventDefault();
      closeMenu(true);
    }
  }

  function menuKeydown(event: KeyboardEvent) {
    const choices = items();
    const current = choices.indexOf(
      document.activeElement as HTMLButtonElement,
    );
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault();
      const next =
        current < 0
          ? event.key === 'ArrowDown'
            ? 0
            : choices.length - 1
          : (current + (event.key === 'ArrowDown' ? 1 : choices.length - 1)) %
            choices.length;
      choices[next]?.focus();
    } else if (event.key === 'Home' || event.key === 'End') {
      event.preventDefault();
      (event.key === 'Home' ? choices[0] : choices.at(-1))?.focus();
    } else if (event.key === 'ArrowRight' || event.key === 'ArrowLeft') {
      event.preventDefault();
      void openMenu(
        ((opened ?? 0) + (event.key === 'ArrowRight' ? 1 : menus.length - 1)) %
          menus.length,
      );
    } else if (event.key === 'Escape') {
      event.preventDefault();
      closeMenu(true);
    } else if (event.key === 'Tab') {
      closeMenu(true);
    } else if (event.ctrlKey || event.metaKey || event.altKey) {
      closeMenu();
    }
  }

  async function select(item: MenuItem) {
    if (item.disabled) return;
    closeMenu(true);
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
  onblur={() => closeMenu()}
/>

<div
  class="menu-bar"
  role="menubar"
  aria-label="Application menu"
  bind:this={root}
>
  {#each menus as menu, index}
    <div class="menu-section" role="none">
      <button
        class="menu-trigger"
        class:open={opened === index}
        role="menuitem"
        id={`${id}-trigger-${index}`}
        aria-haspopup="menu"
        aria-expanded={opened === index}
        aria-controls={opened === index ? `${id}-menu-${index}` : undefined}
        tabindex={focusedMenu === index ? 0 : -1}
        bind:this={triggers[index]}
        onfocus={() => (focusedMenu = index)}
        onclick={() =>
          opened === index && !openedByHover
            ? closeMenu(true)
            : void openMenu(index)}
        onpointerenter={() => {
          if (opened !== null && opened !== index)
            void openMenu(index, 'first', true);
        }}
        onkeydown={(event) => triggerKeydown(event, index)}>{menu.label}</button
      >
      {#if opened === index}
        <div
          class="dropdown-menu"
          role="menu"
          id={`${id}-menu-${index}`}
          aria-labelledby={`${id}-trigger-${index}`}
          tabindex="-1"
          onkeydown={menuKeydown}
        >
          {#each menu.groups as group, groupIndex}
            {#if groupIndex > 0}<div
                class="menu-separator"
                role="separator"
              ></div>{/if}
            {#each group as item}
              <button
                class="menu-item"
                role={item.checked === undefined
                  ? 'menuitem'
                  : 'menuitemcheckbox'}
                aria-checked={item.checked}
                disabled={item.disabled}
                tabindex="-1"
                onclick={() => void select(item)}
                onpointerenter={(event) => {
                  if (!item.disabled) event.currentTarget.focus();
                }}
              >
                <span class="menu-icon">
                  {#if item.checked}<Check size={14} />{:else}<item.icon
                      size={14}
                    />{/if}
                </span>
                <span class="menu-label">{item.label}</span>
                {#if item.key}<kbd>{item.key}</kbd>{/if}
              </button>
            {/each}
          {/each}
        </div>
      {/if}
    </div>
  {/each}
</div>

<style lang="postcss">
  @reference '../styles.css';

  .menu-bar {
    @apply flex gap-[2px] shrink-0;
  }
  .menu-section {
    @apply relative;
  }
  .menu-trigger {
    @apply py-[5px] px-[7px] text-[11px] text-[#969da9] rounded-[4px];
  }
  .menu-trigger:hover,
  .menu-trigger.open {
    @apply bg-[#25262c] text-accent;
  }
  .dropdown-menu {
    @apply absolute top-[calc(100%_+_5px)] left-0 z-[30] w-max min-w-[245px] max-w-[min(360px,_calc(100vw_-_24px))] max-h-[calc(100dvh_-_70px)] overflow-y-auto p-[5px] bg-[#18191d] border border-[#3c3b30] border-t-2 border-t-accent rounded-[6px];
    box-shadow: 0 10px 28px #0008;
  }
  .dropdown-menu:focus {
    outline: none;
  }
  .menu-item {
    @apply flex items-center gap-[9px] w-full min-h-[31px] py-[6px] px-[9px] rounded-[3px] text-left text-[11px] text-[#d4d6dd];
  }
  .menu-item:hover:not(:disabled),
  .menu-item:focus-visible {
    @apply bg-[#f5d54716] text-accent;
    outline: none;
  }
  .menu-icon {
    @apply flex w-[14px] text-[#959ba6];
  }
  .menu-item[aria-checked='true'] .menu-icon {
    @apply text-accent;
  }
  .menu-label {
    @apply flex-1 whitespace-nowrap;
  }
  .menu-item kbd {
    @apply text-[#898e99] ml-[18px] text-[10px];
  }
  .menu-separator {
    @apply h-[1px] my-[5px] mx-[7px] bg-[#303137];
  }
  @media (max-width: 1050px) {
    .menu-trigger {
      @apply p-[5px] text-[10px];
    }
  }
  @media (max-width: 850px) {
    .menu-bar {
      @apply gap-0;
    }
  }
  @media (max-width: 760px) {
    .menu-section:last-child .dropdown-menu {
      @apply left-auto right-0;
    }
  }
</style>
