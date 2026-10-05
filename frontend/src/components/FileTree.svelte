<script module lang="ts">
  import type { Entry } from '../lib/types';
  export type ExplorerEdit = {
    id: string;
    mode: 'file' | 'folder' | 'rename';
    parent: string;
    entry?: Entry;
  };
</script>

<script lang="ts">
  import { tick, untrack, onDestroy } from 'svelte';
  import {
    ChevronRight,
    ChevronDown,
    Folder,
    FolderOpen,
  } from '@lucide/svelte';
  import FileIcon from './FileIcon.svelte';
  import { backend } from '../lib/bridge';

  let {
    root,
    label,
    entries,
    active,
    selected,
    refresh = 0,
    edit = null,
    busy = false,
    onopen,
    onselect,
    onrename,
    oncommit,
    oncancel,
    onmove,
    onerror,
    oncontext,
  }: {
    root: string;
    label: string;
    entries: Entry[];
    active: string;
    selected: string;
    refresh?: number;
    edit?: ExplorerEdit | null;
    busy?: boolean;
    onopen: (entry: Entry) => void;
    onselect: (entry: Entry) => void;
    onrename: (entry: Entry) => void;
    oncommit: (edit: ExplorerEdit, name: string) => Promise<void>;
    oncancel: () => void;
    onmove: (entry: Entry, parent: string) => Promise<void>;
    onerror: (error: unknown) => void;
    oncontext: (entry: Entry, event: MouseEvent) => void;
  } = $props();
  let expanded = $state<Record<string, Entry[]>>({});
  let rootExpanded = $state(true);
  let loading = $state<string[]>([]);
  let name = $state('');
  let error = $state('');
  let submitting = $state(false);
  let inputElement = $state<HTMLInputElement>();
  let treeElement: HTMLDivElement;
  let dragging = $state<Entry | null>(null);
  let dropTarget = $state('');
  let hoverTimer: ReturnType<typeof setTimeout> | undefined;
  let workspaceRoot = '';
  let editingId = '';
  const id = $props.id();
  const rootEntry = $derived({ name: label, path: root, directory: true });
  const rows = $derived.by(() => {
    const result: { entry?: Entry; depth: number; editing?: boolean }[] = [];
    function visit(items: Entry[], depth: number, parent: string) {
      if (edit && edit.mode !== 'rename' && edit.parent === parent)
        result.push({ depth, editing: true });
      for (const entry of items) {
        result.push({
          entry,
          depth,
          editing: edit?.mode === 'rename' && edit.entry?.path === entry.path,
        });
        if (entry.directory && expanded[entry.path])
          visit(expanded[entry.path], depth + 1, entry.path);
      }
    }
    if (rootExpanded) visit(entries, 0, root);
    return result;
  });
  $effect(() => {
    if (root === workspaceRoot) return;
    workspaceRoot = root;
    untrack(() => {
      expanded = {};
      rootExpanded = true;
      clearDrag();
    });
  });
  $effect(() => {
    refresh;
    const workspace = root;
    for (const path of untrack(() => Object.keys(expanded)))
      void backend()
        .ListDirectory(path)
        .then((items) => {
          if (root === workspace && expanded[path])
            expanded = { ...expanded, [path]: items };
        })
        .catch(() => {
          if (root !== workspace) return;
          const next = { ...expanded };
          delete next[path];
          expanded = next;
        });
  });
  $effect(() => {
    const request = edit;
    if (!request) {
      editingId = '';
      return;
    }
    if (editingId === request.id) return;
    editingId = request.id;
    untrack(() => {
      name = request.entry?.name ?? '';
      error = '';
      void reveal(request.parent)
        .then(async () => {
          await tick();
          if (edit?.id !== request.id || !inputElement) return;
          inputElement.focus();
          const extension = request.entry?.directory
            ? -1
            : name.lastIndexOf('.');
          inputElement.setSelectionRange(
            0,
            extension > 0 ? extension : name.length,
          );
        })
        .catch(onerror);
    });
  });
  async function expand(path: string) {
    if (expanded[path]) return;
    const workspace = root;
    loading = [...loading, path];
    try {
      const items = await backend().ListDirectory(path);
      if (root === workspace) expanded = { ...expanded, [path]: items };
    } finally {
      loading = loading.filter((item) => item !== path);
    }
  }
  async function reveal(parent: string) {
    rootExpanded = true;
    if (parent === root || !parent.startsWith(root + '/')) return;
    let path = root;
    for (const segment of parent.slice(root.length + 1).split('/')) {
      path += '/' + segment;
      await expand(path);
    }
  }
  async function toggle(entry: Entry) {
    onselect(entry);
    if (!entry.directory) {
      onopen(entry);
      return;
    }
    if (expanded[entry.path]) {
      const next = { ...expanded };
      delete next[entry.path];
      expanded = next;
    } else
      try {
        await expand(entry.path);
      } catch (error) {
        onerror(error);
      }
  }
  function focusPath(path: string) {
    Array.from(treeElement.querySelectorAll<HTMLButtonElement>('[data-path]'))
      .find((button) => button.dataset.path === path)
      ?.focus();
  }
  function cancelEdit() {
    if (submitting) return;
    oncancel();
    void tick().then(() => focusPath(selected || root));
  }
  async function commit() {
    const request = edit;
    if (!request || submitting) return;
    const value = name.trim();
    if (
      !value ||
      value === '.' ||
      value === '..' ||
      /[/\\\u0000]/.test(value)
    ) {
      error = 'Enter a file or folder name without path separators.';
      inputElement?.focus();
      return;
    }
    submitting = true;
    error = '';
    try {
      await oncommit(request, value);
      if (request.parent !== root) {
        const items = await backend().ListDirectory(request.parent);
        expanded = { ...expanded, [request.parent]: items };
      }
      if (request.mode !== 'file') {
        await tick();
        focusPath(selected);
      }
    } catch (problem) {
      error = String(problem).replace(/^Error: /, '');
      await tick();
      inputElement?.focus();
    } finally {
      submitting = false;
    }
  }
  async function rowKeydown(event: KeyboardEvent, entry: Entry) {
    if (event.key === 'F2' && entry.path !== root) {
      event.preventDefault();
      onrename(entry);
      return;
    }
    const buttons = Array.from(
      treeElement.querySelectorAll<HTMLButtonElement>(
        '[data-path]:not(:disabled)',
      ),
    );
    const index = buttons.indexOf(event.currentTarget as HTMLButtonElement);
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault();
      buttons[
        Math.max(
          0,
          Math.min(
            buttons.length - 1,
            index + (event.key === 'ArrowDown' ? 1 : -1),
          ),
        )
      ]?.focus();
    } else if (event.key === 'Home' || event.key === 'End') {
      event.preventDefault();
      (event.key === 'Home' ? buttons[0] : buttons.at(-1))?.focus();
    } else if (event.key === 'ArrowRight' && entry.directory) {
      event.preventDefault();
      if (entry.path === root) rootExpanded = true;
      else if (!expanded[entry.path]) {
        try {
          await expand(entry.path);
        } catch (error) {
          onerror(error);
        }
      } else buttons[index + 1]?.focus();
    } else if (event.key === 'ArrowLeft') {
      event.preventDefault();
      if (entry.path === root) rootExpanded = false;
      else if (entry.directory && expanded[entry.path]) {
        const next = { ...expanded };
        delete next[entry.path];
        expanded = next;
      } else focusPath(entry.path.slice(0, entry.path.lastIndexOf('/')));
    }
  }
  function clearDrag() {
    clearTimeout(hoverTimer);
    dragging = null;
    dropTarget = '';
  }
  function dragStart(event: DragEvent, entry: Entry) {
    if (busy || edit || !event.dataTransfer) {
      event.preventDefault();
      return;
    }
    dragging = entry;
    onselect(entry);
    event.dataTransfer.setData('application/x-praewa-entry', entry.path);
    event.dataTransfer.setData('text/plain', entry.path);
    event.dataTransfer.effectAllowed = 'move';
  }
  function canDrop(parent: string) {
    return (
      !!dragging &&
      !busy &&
      !edit &&
      parent !== dragging.path &&
      !parent.startsWith(dragging.path + '/') &&
      parent !== dragging.path.slice(0, dragging.path.lastIndexOf('/'))
    );
  }
  function dragOver(event: DragEvent, parent: string) {
    event.stopPropagation();
    if (!canDrop(parent)) {
      if (event.dataTransfer) event.dataTransfer.dropEffect = 'none';
      return;
    }
    event.preventDefault();
    if (event.dataTransfer) event.dataTransfer.dropEffect = 'move';
    if (dropTarget !== parent) {
      clearTimeout(hoverTimer);
      dropTarget = parent;
      if (parent !== root && !expanded[parent])
        hoverTimer = setTimeout(() => {
          void expand(parent).catch(onerror);
        }, 450);
    }
  }
  function dragLeave(event: DragEvent, parent: string) {
    if (
      event.relatedTarget instanceof Node &&
      (event.currentTarget as HTMLElement).contains(event.relatedTarget)
    )
      return;
    if (dropTarget === parent) {
      dropTarget = '';
      clearTimeout(hoverTimer);
    }
  }
  async function drop(event: DragEvent, parent: string) {
    event.stopPropagation();
    if (!canDrop(parent) || !dragging) return;
    event.preventDefault();
    const entry = dragging;
    clearDrag();
    try {
      await onmove(entry, parent);
      await reveal(parent);
      if (parent !== root)
        expanded = {
          ...expanded,
          [parent]: await backend().ListDirectory(parent),
        };
      await tick();
      focusPath(selected);
    } catch (error) {
      onerror(error);
    }
  }
  onDestroy(() => clearTimeout(hoverTimer));
</script>

<div
  class="file-tree"
  role="tree"
  aria-label="Workspace files"
  bind:this={treeElement}
>
  <button
    class="project-heading"
    class:drop-target={dropTarget === root}
    role="treeitem"
    aria-expanded={rootExpanded}
    aria-selected={selected === root}
    aria-level="1"
    data-path={root}
    title={root}
    onclick={() => {
      onselect(rootEntry);
      rootExpanded = !rootExpanded;
    }}
    onkeydown={(event) => void rowKeydown(event, rootEntry)}
    oncontextmenu={(event) => {
      event.preventDefault();
      oncontext(rootEntry, event);
    }}
    ondragover={(event) => dragOver(event, root)}
    ondragleave={(event) => dragLeave(event, root)}
    ondrop={(event) => void drop(event, root)}
    >{#if rootExpanded}<ChevronDown size={13} />{:else}<ChevronRight
        size={13}
      />{/if}<span>{label}</span></button
  >
  <div
    class="tree-content"
    class:drop-target={dropTarget === root}
    role="group"
    ondragover={(event) => dragOver(event, root)}
    ondragleave={(event) => dragLeave(event, root)}
    ondrop={(event) => void drop(event, root)}
  >
    {#each rows as row (row.editing ? edit!.id : row.entry!.path)}
      {#if row.editing && edit}
        <div
          class="inline-entry"
          style:padding-left={`${32 + row.depth * 14}px`}
        >
          {#if edit.mode === 'folder' || edit.entry?.directory}<Folder
              size={16}
              class="folder-icon"
            />{:else}<FileIcon name={name || 'untitled'} />{/if}
          <div class="inline-name">
            <input
              class:invalid={!!error}
              bind:this={inputElement}
              bind:value={name}
              aria-label={edit.mode === 'rename'
                ? 'Rename entry'
                : edit.mode === 'folder'
                  ? 'New folder name'
                  : 'New file name'}
              aria-invalid={!!error}
              aria-describedby={error ? `${id}-error` : undefined}
              placeholder={edit.mode === 'folder' ? 'Folder name' : 'File name'}
              disabled={submitting}
              autocomplete="off"
              spellcheck="false"
              oninput={() => (error = '')}
              onkeydown={(event) => {
                if (event.key === 'Enter' || event.key === 'Escape') {
                  event.preventDefault();
                  event.stopPropagation();
                  if (event.key === 'Enter') void commit();
                  else cancelEdit();
                }
              }}
              onblur={() => {
                if (submitting || !edit) return;
                if (name.trim()) void commit();
                else cancelEdit();
              }}
            />
            {#if error}<div
                class="inline-error"
                id={`${id}-error`}
                role="alert"
              >
                {error}
              </div>{/if}
          </div>
        </div>
      {:else if row.entry}
        {@const entry = row.entry}
        <button
          class="tree-entry"
          class:active={(selected || active) === entry.path}
          class:drop-target={dropTarget === entry.path}
          class:dragging={dragging?.path === entry.path}
          role="treeitem"
          aria-level={row.depth + 2}
          aria-selected={(selected || active) === entry.path}
          aria-expanded={entry.directory ? !!expanded[entry.path] : undefined}
          data-path={entry.path}
          style:padding-left={`${12 + row.depth * 14}px`}
          oncontextmenu={(event) => {
            event.preventDefault();
            oncontext(entry, event);
          }}
          onclick={() => void toggle(entry)}
          onfocus={() => onselect(entry)}
          onkeydown={(event) => void rowKeydown(event, entry)}
          title={entry.path}
          disabled={loading.includes(entry.path)}
          draggable={!busy && !edit}
          ondragstart={(event) => dragStart(event, entry)}
          ondragend={clearDrag}
          ondragover={(event) =>
            dragOver(
              event,
              entry.directory
                ? entry.path
                : entry.path.slice(0, entry.path.lastIndexOf('/')),
            )}
          ondragleave={(event) => dragLeave(event, entry.path)}
          ondrop={(event) =>
            void drop(
              event,
              entry.directory
                ? entry.path
                : entry.path.slice(0, entry.path.lastIndexOf('/')),
            )}
        >
          {#if entry.directory}{#if expanded[entry.path]}<ChevronDown
                size={13}
              /><FolderOpen size={15} class="folder-icon" />{:else}<ChevronRight
                size={13}
              /><Folder size={15} class="folder-icon" />{/if}
          {:else}<span class="tree-spacer"></span><FileIcon
              name={entry.name}
            />{/if}<span>{entry.name}</span>
        </button>
      {/if}
    {/each}
  </div>
</div>

<style lang="postcss">
  @reference '../styles.css';

  .project-heading {
    @apply w-full text-left;
  }
  .tree-content {
    @apply min-h-[30px] pb-[10px];
  }
  .drop-target {
    @apply bg-[#f5d54718];
    box-shadow: inset 0 0 0 1px #f5d54780;
  }
  .tree-entry.dragging {
    @apply opacity-[0.45];
  }
  .inline-entry {
    @apply flex gap-[7px] items-start pt-[3px] pr-[10px] pb-[3px] pl-[32px];
  }
  .inline-entry :global(svg),
  .inline-entry :global(img) {
    @apply mt-[4px];
  }
  .inline-name {
    @apply flex-1 min-w-0;
  }
  .inline-name input {
    @apply w-full h-[24px] py-[3px] px-[5px] bg-[#101114] text-[#e2e4e8] border border-accent;
    outline: none;
    @apply rounded-[2px] text-[11px];
  }
  .inline-name input.invalid {
    @apply border-[#e47a78];
  }
  .inline-error {
    @apply text-[#e8a2a0] border border-[#924c4b] bg-[#312020] text-[10px] leading-[1.5] p-[5px] wrap-anywhere;
  }
</style>
