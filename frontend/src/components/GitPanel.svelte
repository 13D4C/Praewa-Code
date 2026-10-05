<script lang="ts">
  import {
    Plus,
    Minus,
    GitCommitHorizontal,
    GitBranch,
    RefreshCw,
  } from '@lucide/svelte';
  import type { GitState } from '../lib/types';
  import FileIcon from './FileIcon.svelte';
  let {
    gitState,
    root,
    busy,
    onstage,
    ondiff,
    oncommit,
    oninit,
    onrefresh,
  }: {
    gitState: GitState | null;
    root: string;
    busy: boolean;
    onstage: (path: string, staged: boolean) => void;
    ondiff: (path: string, staged: boolean) => void;
    oncommit: (message: string) => Promise<boolean>;
    oninit: () => void;
    onrefresh: () => void;
  } = $props();
  let message = $state('');
  const staged = $derived(
    gitState?.files.filter(
      (file) => file.index !== ' ' && file.index !== '?',
    ) ?? [],
  );
  const changed = $derived(
    gitState?.files.filter((file) => file.worktree !== ' ') ?? [],
  );
</script>

<div class="panel-form">
  <div class="panel-caption git-branch">
    <GitBranch size={14} />{gitState?.branch || 'Source control'}<button
      class="icon-button"
      title="Refresh Git"
      onclick={onrefresh}><RefreshCw size={13} /></button
    >
  </div>
  {#if gitState?.problem}<p class="form-error">{gitState.problem}</p>{/if}
  {#if !gitState?.repository}<p class="panel-caption">
      {root
        ? 'This workspace has no Git repository.'
        : 'Open a workspace to use Git.'}
    </p>
    <button
      class="secondary-button full-width"
      disabled={!root || !gitState?.available || busy}
      onclick={oninit}>Initialize repository</button
    >{:else}
    <textarea
      class="small-input commit-input"
      aria-label="Commit message"
      placeholder="Commit message"
      bind:value={message}></textarea>
    <button
      class="primary-button full-width"
      disabled={!message.trim() || !staged.length || busy}
      onclick={async () => {
        if (await oncommit(message)) message = '';
      }}
      ><GitCommitHorizontal size={14} />Commit staged ({staged.length})</button
    >
  {/if}
</div>
<div class="panel-scroll">
  {#each [{ label: 'STAGED CHANGES', files: staged, stage: true }, { label: 'CHANGES', files: changed, stage: false }] as group}
    <div class="list-section">
      {group.label}<span>{group.files.length}</span>
    </div>
    {#each group.files as file}<div class="git-file">
        <button
          class="git-file-name"
          onclick={() => ondiff(file.path, group.stage)}
          title={file.path}
          ><FileIcon name={file.path} /><span
            >{file.path.slice(root.length + 1)}</span
          ><b
            >{group.stage
              ? file.index
              : file.worktree === '?'
                ? 'U'
                : file.worktree}</b
          ></button
        ><button
          class="icon-button"
          disabled={busy}
          title={group.stage ? 'Unstage file' : 'Stage file'}
          onclick={() => onstage(file.path, !group.stage)}
          >{#if group.stage}<Minus size={14} />{:else}<Plus
              size={14}
            />{/if}</button
        >
      </div>{/each}
  {/each}
  {#if gitState?.repository && !gitState.files.length}<p
      class="panel-caption panel-form"
    >
      Working tree clean.
    </p>{/if}
  {#if gitState?.log}<div class="list-section">RECENT COMMITS</div>
    <pre class="git-log">{gitState.log}</pre>{/if}
</div>
