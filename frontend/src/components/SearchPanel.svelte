<script lang="ts">
  import { Search, ReplaceAll, RefreshCw } from '@lucide/svelte';
  import { backend } from '../lib/bridge';
  import FileIcon from './FileIcon.svelte';
  import type { SearchQuery, SearchResult, SearchMatch } from '../lib/types';
  let {
    root,
    revision,
    onopen,
    onreplace,
    onerror,
  }: {
    root: string;
    revision: number;
    onopen: (match: SearchMatch) => void;
    onreplace: (
      query: SearchQuery,
      replacement: string,
      matches: SearchMatch[],
    ) => void;
    onerror: (error: unknown) => void;
  } = $props();
  let text = $state('');
  let replacement = $state('');
  let include = $state('');
  let regex = $state(false);
  let caseSensitive = $state(false);
  let wholeWord = $state(false);
  let result = $state<SearchResult>({ matches: [], truncated: false });
  let busy = $state(false);
  let problem = $state('');
  let sequence = 0;
  const groups = $derived([
    ...new Set(result.matches.map((match) => match.path)),
  ]);
  function query(): SearchQuery {
    return { text, regex, caseSensitive, wholeWord, include };
  }
  async function search() {
    const id = ++sequence;
    if (!text || !root) {
      result = { matches: [], truncated: false };
      return;
    }
    busy = true;
    problem = '';
    try {
      const next = await backend().Search(query());
      if (id === sequence) result = next;
    } catch (error) {
      if (id === sequence) problem = String(error);
    } finally {
      if (id === sequence) busy = false;
    }
  }
  $effect(() => {
    text;
    regex;
    caseSensitive;
    wholeWord;
    include;
    revision;
    root;
    const timer = setTimeout(() => {
      void search();
    }, 300);
    return () => clearTimeout(timer);
  });
</script>

<div class="panel-form">
  <div class="input-with-icon">
    <Search size={14} /><input
      aria-label="Search project"
      placeholder="Search project"
      bind:value={text}
      onkeydown={(event) => {
        if (event.key === 'Enter') void search();
      }}
    />
  </div>
  <div class="search-options">
    <button
      class:enabled={caseSensitive}
      onclick={() => (caseSensitive = !caseSensitive)}
      title="Match case">Aa</button
    ><button
      class:enabled={wholeWord}
      onclick={() => (wholeWord = !wholeWord)}
      title="Whole word">ab</button
    ><button
      class:enabled={regex}
      onclick={() => (regex = !regex)}
      title="Regular expression">.*</button
    ><button class="icon-button" title="Search again" onclick={() => search()}
      ><RefreshCw size={13} /></button
    >
  </div>
  <input
    class="small-input"
    aria-label="Replace with"
    placeholder="Replace with"
    bind:value={replacement}
  />
  <input
    class="small-input"
    aria-label="Filter file path"
    placeholder="Filter file path (e.g. src/)"
    bind:value={include}
  />
  <button
    class="secondary-button full-width"
    disabled={!result.matches.length || busy}
    onclick={() => onreplace(query(), replacement, result.matches)}
    ><ReplaceAll size={13} />Replace all…</button
  >
  <p class="panel-caption">
    {busy
      ? 'Searching…'
      : `${result.matches.length}${result.truncated ? '+' : ''} matches in ${groups.length} files`}
  </p>
  {#if problem}<p class="form-error">{problem}</p>{/if}
  {#if !root}<p class="panel-caption">
      Open a workspace to search its files.
    </p>{/if}
</div>
<div class="panel-scroll">
  {#each groups as path}<div class="search-file">
      <FileIcon name={path} /><span>{path.slice(root.length + 1)}</span>
    </div>
    {#each result.matches.filter((match) => match.path === path) as match, i (`${path}:${match.line}:${match.column}:${i}`)}<button
        class="search-hit"
        onclick={() => onopen(match)}
        title={match.text}
        ><span>{match.line}</span><code>{match.text.trim()}</code></button
      >{/each}{/each}
</div>
