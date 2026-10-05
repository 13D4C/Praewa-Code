<script lang="ts">
  import { tick, untrack } from 'svelte';
  import {
    Code2,
    PencilRuler,
    MessageCircle,
    ScanSearch,
    ChevronDown,
    Check,
    Plus,
    Settings2,
    ArrowLeft,
    FileText,
    Search,
    X,
    RefreshCw,
    FolderOpen,
    Import,
  } from '@lucide/svelte';
  import FileIcon from './FileIcon.svelte';
  import {
    behaviors,
    copyMode,
    type AgentMode,
    type AgentSkill,
  } from '../lib/agent-modes';

  let {
    workspace,
    files,
    modes,
    selected,
    visible,
    editing = $bindable(false),
    onselect,
    onsave,
    onreadfile,
  }: {
    workspace: string;
    files: { name: string; path: string }[];
    modes: AgentMode[];
    selected: AgentMode;
    visible: boolean;
    editing?: boolean;
    onselect: (id: string) => void;
    onsave: (mode: AgentMode) => void;
    onreadfile: (path: string) => Promise<string>;
  } = $props();
  let menuOpen = $state(false);
  let root: HTMLDivElement;
  let modeNameInput = $state<HTMLInputElement>();
  let importInput = $state<HTMLInputElement>();
  let form = $state<AgentMode | null>(null);
  let isNew = $state(false);
  let query = $state('');
  let error = $state('');
  let busy = $state(false);
  let revision = 0;
  const icons = {
    code: Code2,
    plan: PencilRuler,
    ask: MessageCircle,
    review: ScanSearch,
    chat: MessageCircle,
  };
  const SelectedIcon = $derived(icons[selected.behavior]);
  const skillFiles = $derived(
    files
      .filter(
        (file) =>
          /\.md$/i.test(file.name) &&
          `${file.name} ${relative(file.path)}`
            .toLowerCase()
            .includes(query.toLowerCase()),
      )
      .sort(
        (a, b) =>
          Number(/^skills?\.md$/i.test(b.name)) -
            Number(/^skills?\.md$/i.test(a.name)) ||
          a.path.localeCompare(b.path),
      )
      .slice(0, 50),
  );

  $effect(() => {
    workspace;
    untrack(() => {
      revision++;
      menuOpen = false;
      editing = false;
      form = null;
      busy = false;
      error = '';
    });
  });
  $effect(() => {
    if (!visible) menuOpen = false;
  });

  function relative(path: string) {
    if (path.startsWith('untitled:')) return 'Unsaved editor buffer';
    return workspace && path.startsWith(workspace + '/')
      ? path.slice(workspace.length + 1)
      : path;
  }
  function begin(create = false) {
    revision++;
    menuOpen = false;
    isNew = create;
    query = '';
    error = '';
    busy = false;
    form = create
      ? {
          id: crypto.randomUUID(),
          name: '',
          description: '',
          behavior: 'code',
          instructions: '',
          skills: [],
        }
      : copyMode(selected);
    editing = true;
    void tick().then(() => modeNameInput?.focus());
  }
  function cancel() {
    revision++;
    editing = false;
    busy = false;
    error = '';
  }
  function save() {
    if (!form || busy) return;
    const name = form.name.trim();
    if (!name) {
      error = 'Give this mode a name.';
      modeNameInput?.focus();
      return;
    }
    if (
      modes.some(
        (item) =>
          item.id !== form?.id &&
          item.name.toLowerCase() === name.toLowerCase(),
      )
    ) {
      error = 'A mode with this name already exists.';
      modeNameInput?.focus();
      return;
    }
    onsave(
      copyMode({
        ...form,
        name,
        description: form.description.trim(),
        instructions: form.instructions.trim(),
      }),
    );
    cancel();
  }
  function saveShortcut(event: KeyboardEvent) {
    if (
      !event.isComposing &&
      (event.ctrlKey || event.metaKey) &&
      (event.key === 'Enter' || event.key.toLowerCase() === 's')
    ) {
      event.preventDefault();
      event.stopPropagation();
      save();
    }
  }
  function checkContent(name: string, content: string) {
    if (!/\.md$/i.test(name))
      throw new Error('Choose a Markdown skill file (.md).');
    if (new TextEncoder().encode(content).length > 256 * 1024)
      throw new Error(
        `${name} is too large. Choose a skill file under 256 KB.`,
      );
    if (!content.trim())
      throw new Error(
        `${name} is empty. Add instructions before selecting it.`,
      );
    return content;
  }
  async function addWorkspace(
    file: { name: string; path: string },
    refresh = false,
  ) {
    if (!form || busy) return;
    if (!refresh && form.skills.some((skill) => skill.path === file.path)) {
      form.skills = form.skills.filter((skill) => skill.path !== file.path);
      return;
    }
    const token = revision;
    busy = true;
    error = '';
    try {
      const content = checkContent(file.name, await onreadfile(file.path));
      if (token !== revision || !editing || !form) return;
      const skill: AgentSkill = { ...file, source: 'workspace', content };
      const index = form.skills.findIndex((item) => item.path === file.path);
      if (index >= 0) form.skills[index] = skill;
      else form.skills.push(skill);
    } catch (reason) {
      if (token === revision && editing)
        error = reason instanceof Error ? reason.message : String(reason);
    } finally {
      if (token === revision) busy = false;
    }
  }
  async function importSkills(event: Event) {
    const input = event.currentTarget as HTMLInputElement;
    const chosen = Array.from(input.files ?? []);
    input.value = '';
    if (!form || busy || !chosen.length) return;
    const token = revision;
    const target = form;
    busy = true;
    error = '';
    const imported: AgentSkill[] = [];
    const problems: string[] = [];
    for (const file of chosen) {
      try {
        if (file.size > 256 * 1024)
          throw new Error(
            `${file.name} is too large. Choose a skill file under 256 KB.`,
          );
        const content = checkContent(file.name, await file.text());
        imported.push({
          name: file.name,
          path: `imported:${crypto.randomUUID()}`,
          content,
          source: 'imported',
        });
      } catch (reason) {
        problems.push(
          reason instanceof Error ? reason.message : String(reason),
        );
      }
    }
    if (token !== revision || !editing || form !== target) return;
    for (const skill of imported) {
      const existing = form.skills.findIndex(
        (item) => item.source === 'imported' && item.name === skill.name,
      );
      if (existing >= 0)
        form.skills[existing] = { ...skill, path: form.skills[existing].path };
      else form.skills.push(skill);
    }
    error = problems.join(' ');
    busy = false;
  }
  function outside(event: Event) {
    if (event.target instanceof Node && !root?.contains(event.target))
      menuOpen = false;
  }
</script>

<svelte:window
  onpointerdown={outside}
  onkeydown={(event) => {
    if (event.key === 'Escape' && menuOpen) {
      event.preventDefault();
      menuOpen = false;
    }
  }}
/>

<div class="agent-modes" class:editing bind:this={root}>
  {#if editing && form}
    <div class="mode-editor-heading">
      <button
        class="icon-button"
        aria-label="Back to agent chat"
        title="Cancel changes"
        onclick={cancel}><ArrowLeft size={16} /></button
      >
      <strong>{isNew ? 'Create mode' : 'Edit mode'}</strong>
      <span>{isNew ? 'CUSTOM' : 'SETTINGS'}</span>
    </div>
    <form
      class="mode-form"
      onsubmit={(event) => {
        event.preventDefault();
        save();
      }}
    >
      <div class="mode-fields">
        <label
          >Mode name<input
            bind:this={modeNameInput}
            aria-label="Mode name"
            bind:value={form.name}
            placeholder="e.g. Svelte specialist"
            maxlength="80"
            onkeydown={saveShortcut}
          /></label
        >
        <label
          >Description<input
            aria-label="Mode description"
            bind:value={form.description}
            placeholder="What should this mode help with?"
            maxlength="240"
            onkeydown={saveShortcut}
          /></label
        >
        <label
          >Base behavior<select
            aria-label="Mode behavior"
            bind:value={form.behavior}
            onkeydown={saveShortcut}
            >{#each behaviors as item}<option value={item.id}
                >{item.name}</option
              >{/each}</select
          ></label
        >
        <label
          >Instructions<textarea
            aria-label="Mode instructions"
            bind:value={form.instructions}
            placeholder="Describe how the agent should work in this mode…"
            maxlength="20000"
            onkeydown={saveShortcut}></textarea></label
        >
        <div class="skill-heading">
          <span
            ><FileText size={13} />Skill files
            <small>{form.skills.length}</small></span
          ><button
            type="button"
            class="import-skills"
            disabled={busy}
            onclick={() => importInput?.click()}
            ><Import size={12} />Import .md</button
          >
        </div>
        <input
          class="file-input"
          bind:this={importInput}
          type="file"
          accept=".md,text/markdown"
          multiple
          aria-label="Import skill files"
          onchange={importSkills}
        />
        {#each form.skills as skill (skill.path)}
          <div class="selected-skill">
            <div class="skill-summary">
              <FileIcon name={skill.name} size={14} /><span
                >{skill.name}<small
                  >{skill.source === 'bundled'
                    ? 'Built-in skill'
                    : skill.source === 'imported'
                      ? 'Imported copy'
                      : relative(skill.path)}</small
                ></span
              >
              {#if skill.source === 'workspace'}<button
                  type="button"
                  class="icon-button"
                  disabled={busy}
                  aria-label={`Refresh skill ${skill.name}`}
                  title="Read the latest file contents"
                  onclick={() => addWorkspace(skill, true)}
                  ><RefreshCw size={12} /></button
                >{/if}
              <button
                type="button"
                class="icon-button"
                disabled={busy}
                aria-label={`Remove skill ${skill.name}`}
                onclick={() => {
                  if (form)
                    form.skills = form.skills.filter(
                      (item) => item.path !== skill.path,
                    );
                }}><X size={13} /></button
              >
            </div>
            <details>
              <summary>Preview instructions</summary>
              <pre>{skill.content}</pre>
            </details>
          </div>
        {/each}
        <p class="skill-hint">
          Choose SKILL.md, skills.md or another Markdown file. This mode keeps a
          copy of its instructions.
        </p>
        <div class="skill-search">
          <Search size={13} /><input
            aria-label="Find skill files"
            bind:value={query}
            placeholder="Find a skill in this project…"
            onkeydown={saveShortcut}
          />
        </div>
        <div class="workspace-skills" aria-label="Workspace skill files">
          {#each skillFiles as file}<button
              type="button"
              class:chosen={form.skills.some((item) => item.path === file.path)}
              disabled={busy}
              aria-pressed={form.skills.some((item) => item.path === file.path)}
              onclick={() => addWorkspace(file)}
              title={relative(file.path)}
            >
              <FileIcon name={file.name} size={15} /><span
                >{file.name}<small>{relative(file.path)}</small></span
              ><span class="skill-check"
                >{#if form.skills.some((item) => item.path === file.path)}<Check
                    size={12}
                  />{:else}<Plus size={12} />{/if}</span
              >
            </button>{/each}
          {#if !skillFiles.length}<div class="no-skills">
              <FolderOpen size={17} /><span
                >{query
                  ? 'No matching Markdown files'
                  : 'No Markdown files in this project'}<small
                  >Open a project or import a skill file.</small
                ></span
              >
            </div>{/if}
        </div>
      </div>
      <footer class="mode-form-footer">
        {#if error}<p role="alert">{error}</p>{/if}
        <div>
          <span
            >{busy
              ? 'Reading skill file…'
              : 'Saved locally for this project'}</span
          ><button class="save-mode" type="submit" disabled={busy}
            >{isNew ? 'Create mode' : 'Save mode'}<Check size={13} /></button
          >
        </div>
      </footer>
    </form>
  {:else}
    <div class="mode-bar">
      <button
        class="mode-trigger"
        aria-label="Choose agent mode"
        aria-expanded={menuOpen}
        title={`${selected.name}: ${selected.description}`}
        onclick={() => (menuOpen = !menuOpen)}
        ><SelectedIcon size={14} /><span>{selected.name}</span><ChevronDown
          size={12}
        /></button
      >
      <button
        class="skill-count"
        aria-label="Configure mode skills"
        title="Choose skill files for this mode"
        onclick={() => begin()}
        ><FileText size={12} /><span
          >{selected.skills.length} skill{selected.skills.length === 1
            ? ''
            : 's'}</span
        ></button
      >
      <button
        class="icon-button mode-settings"
        aria-label="Edit current mode"
        title="Edit mode"
        onclick={() => begin()}><Settings2 size={14} /></button
      >
    </div>
    {#if menuOpen}<div class="mode-menu" aria-label="Agent modes">
        <div class="menu-label">CHOOSE MODE</div>
        <div class="mode-options">
          {#each modes as item}{@const Icon = icons[item.behavior]}<button
              class:selected={selected.id === item.id}
              aria-pressed={selected.id === item.id}
              onclick={() => {
                onselect(item.id);
                menuOpen = false;
              }}
              ><Icon size={16} /><span
                >{item.name}<small
                  >{item.description ||
                    behaviors.find((behavior) => behavior.id === item.behavior)
                      ?.description}</small
                ></span
              >{#if selected.id === item.id}<Check size={13} />{/if}</button
            >{/each}
        </div>
        <button class="create-mode" onclick={() => begin(true)}
          ><Plus size={14} />Create mode</button
        >
      </div>{/if}
  {/if}
</div>

<style lang="postcss">
  @reference '../styles.css';

  .agent-modes {
    @apply relative shrink-0;
    color-scheme: dark;
    scrollbar-color: #37383e transparent;
  }
  .agent-modes.editing {
    @apply flex flex-col flex-1 min-h-0 overflow-hidden;
  }
  .mode-bar {
    @apply flex items-center gap-[6px] pt-[1px] pr-[13px] pb-[10px] pl-[13px] border-b border-b-[#24262b];
  }
  .mode-trigger {
    @apply flex items-center gap-[6px] py-[6px] px-[8px] bg-[#f5d5470c] border border-[#f5d54730] rounded-[5px] text-[#e0cb7a] text-[11px] min-w-0 max-w-[60%];
  }
  .mode-trigger > span {
    @apply overflow-hidden text-ellipsis whitespace-nowrap;
  }
  .mode-trigger :global(svg) {
    @apply shrink-0;
  }
  .mode-trigger:hover {
    @apply bg-[#f5d54718];
  }
  .skill-count {
    @apply flex items-center gap-[4px] p-[5px] text-[10px] text-[#8a919f] whitespace-nowrap;
  }
  .skill-count:hover {
    @apply text-[#e0cb7a];
  }
  .mode-settings {
    @apply ml-auto;
  }
  .mode-menu {
    @apply absolute top-[35px] left-[13px] right-[13px] z-[28] p-[5px] bg-[#191a1e] border border-[#49412a];
    box-shadow: 0 10px 28px #0008;
    @apply rounded-[6px];
  }
  .menu-label {
    @apply text-[#7e8592] text-[9px] tracking-[0.6px] py-[7px] px-[8px];
  }
  .mode-options {
    @apply max-h-[260px] overflow-y-auto;
  }
  .mode-options button {
    @apply flex items-center gap-[9px] w-full py-[10px] px-[8px] text-left rounded-[4px] text-[11px];
  }
  .mode-options button > span {
    @apply flex-1 min-w-0 wrap-anywhere;
  }
  .mode-options button :global(svg) {
    @apply shrink-0 text-[#9198a6];
  }
  .mode-options small {
    @apply block text-[#7f8795] text-[9px] mt-[4px] leading-[1.5];
  }
  .mode-options button:hover,
  .mode-options button.selected {
    @apply bg-[#f5d54710] text-[#ecd688];
  }
  .mode-options button.selected :global(svg) {
    @apply text-[#ecd688];
  }
  .create-mode {
    @apply flex items-center gap-[8px] w-full pt-[10px] pr-[8px] pb-[7px] pl-[8px] border-t border-t-[#313239] mt-[4px] text-[11px] text-[#e0cb7a] text-left;
  }
  .create-mode:hover {
    @apply bg-[#f5d54710];
  }
  .mode-editor-heading {
    @apply flex items-center gap-[8px] pt-[4px] pr-[13px] pb-[10px] pl-[13px] border-b border-b-[#2c2e35] shrink-0;
  }
  .mode-editor-heading strong {
    @apply text-[12px] font-medium;
  }
  .mode-editor-heading > span {
    @apply ml-auto text-[#827b61] text-[8px] tracking-[0.7px];
  }
  .mode-form {
    @apply flex flex-col min-h-0 flex-1;
  }
  .mode-fields {
    @apply p-[13px] overflow-y-auto flex-1 min-h-0;
    scrollbar-width: thin;
  }
  label {
    @apply block text-[10px] text-[#a0a7b4] mb-[12px];
  }
  label input,
  label select,
  label textarea {
    @apply block mt-[6px] w-full bg-[#191b1f] border border-[#32353d] text-[#d6dae2] rounded-[5px] p-[8px];
    font: inherit;
    @apply text-[11px];
    outline: none;
  }
  label input:focus,
  label select:focus,
  label textarea:focus {
    @apply border-[#b7a24e];
  }
  label textarea {
    @apply min-h-[90px] resize-y leading-[1.6];
  }
  label input::placeholder,
  label textarea::placeholder {
    @apply text-[#656d7a];
  }
  .skill-heading,
  .skill-heading > span {
    @apply flex items-center gap-[6px];
  }
  .skill-heading {
    @apply justify-between text-[11px] text-[#c0c5ce] mt-[18px] mr-0 mb-[10px] ml-0;
  }
  .skill-heading small {
    @apply py-[2px] px-[5px] border border-[#3a3d45] rounded-[4px] text-[9px] text-[#9198a6];
  }
  .import-skills {
    @apply flex items-center gap-[5px] border border-[#3e3b2b] rounded-[4px] py-[5px] px-[6px] text-[#d7c47b] text-[10px];
  }
  .import-skills:hover {
    @apply bg-[#f5d54710];
  }
  .file-input {
    @apply hidden;
  }
  .skill-hint {
    @apply text-[9px] leading-[1.6] text-[#767f8e] mt-[9px] mr-0 mb-[12px] ml-0;
  }
  .skill-search {
    @apply flex items-center gap-[6px] p-[8px] bg-[#181a1e] border border-[#32353d] rounded-[5px_5px_0_0] text-[#7c8492];
  }
  .skill-search input {
    @apply flex-1 min-w-0;
    outline: none;
    @apply border-0 bg-transparent text-[#d5d9e1] text-[10px];
  }
  .skill-search:focus-within {
    @apply border-[#b7a24e];
  }
  .workspace-skills {
    @apply p-[4px] bg-[#151619] border border-[#32353d] border-t-0 rounded-[0_0_5px_5px] max-h-[190px] overflow-y-auto;
    scrollbar-width: thin;
  }
  .workspace-skills button {
    @apply flex items-center gap-[7px] w-full text-left py-[8px] px-[6px] rounded-[4px] text-[10px];
  }
  .workspace-skills button:hover,
  .workspace-skills button.chosen {
    @apply bg-[#f5d5470c];
  }
  .workspace-skills button > span:not(.skill-check),
  .skill-summary > span {
    @apply flex-1 min-w-0 wrap-anywhere;
  }
  .workspace-skills small,
  .skill-summary small {
    @apply block mt-[3px] text-[9px] text-[#747f90];
  }
  .skill-check {
    @apply shrink-0 w-[17px] h-[17px] border border-[#484a50] flex items-center justify-center rounded-[3px] text-[#8992a0];
  }
  .chosen .skill-check {
    @apply border-[#b7a24e] bg-[#f5d54712] text-[#e0cb7a];
  }
  .no-skills {
    @apply flex items-center gap-[9px] text-[#838c9b] text-[10px] py-[16px] px-[8px];
  }
  .no-skills small {
    @apply text-[9px] text-[#656e7c];
  }
  .selected-skill {
    @apply border border-[#46412c] bg-[#f5d54705] rounded-[5px] mb-[7px] overflow-hidden;
  }
  .skill-summary {
    @apply flex items-center gap-[6px] p-[8px] text-[10px];
  }
  .skill-summary .icon-button {
    @apply w-[22px] h-[22px] shrink-0;
  }
  details {
    @apply border-t border-t-[#3b3828];
  }
  summary {
    @apply py-[7px] px-[9px] text-[9px] text-[#9b936d] cursor-pointer;
  }
  pre {
    @apply max-h-[150px] overflow-auto m-0 py-[8px] px-[10px] text-[#9da7b8] text-[9px] leading-[1.6] whitespace-pre-wrap wrap-anywhere bg-surface;
  }
  .mode-form-footer {
    @apply py-[12px] px-[13px] border-t border-t-[#2c2e35] bg-[#141519] shrink-0;
  }
  .mode-form-footer > div {
    @apply flex items-center justify-between gap-[8px];
  }
  .mode-form-footer > div > span {
    @apply text-[#757e8d] text-[9px] leading-[1.5];
  }
  .mode-form-footer p {
    @apply text-[#e5a28d] text-[10px] leading-[1.5] mt-0 mr-0 mb-[10px] ml-0;
  }
  .save-mode {
    @apply flex items-center gap-[6px] bg-accent text-[#17180f] py-[7px] px-[9px] rounded-[5px] text-[10px] font-semibold whitespace-nowrap;
  }
  .save-mode:hover:not(:disabled) {
    @apply bg-[#ffe576];
  }
</style>
