<script module lang="ts">
  export type ChatFile = { name: string; path: string };
  export type ChatDraft = {
    id: string;
    text: string;
    files: ChatFile[];
    createdAt: string;
    mode?: import('../lib/agent-modes').AgentMode;
  };
  export type DraftChat = {
    id: string;
    title: string;
    messages: ChatDraft[];
    composer: string;
    files: ChatFile[];
    modeId: string;
    threadId: string;
  };
</script>

<script lang="ts">
  import { tick, untrack, onDestroy, onMount } from 'svelte';
  import {
    Bot,
    Plus,
    X,
    History,
    Paperclip,
    ArrowUp,
    FolderOpen,
    Search,
    FileCode2,
    Sparkles,
    Bug,
    PencilRuler,
    Circle,
    RefreshCw,
    Square,
  } from '@lucide/svelte';
  import FileIcon from './FileIcon.svelte';
  import AgentModes from './AgentModes.svelte';
  import AgentRequests from './AgentRequests.svelte';
  import MarkdownPreview from './MarkdownPreview.svelte';
  import SixSevenChallenge from './SixSevenChallenge.svelte';
  import {
    PromptChallenge,
    SIX_SEVEN_TARGET,
    SIX_SEVEN_MIN_TARGET,
    SIX_SEVEN_MAX_TARGET,
    SIX_SEVEN_TARGET_STORAGE,
    parseSixSevenTarget,
  } from '../lib/six-seven';
  import {
    agentBackend,
    onAgentEvent,
    type AgentConnection,
    type AgentConversation,
    type AgentThread,
  } from '../lib/agent';
  import {
    defaultModes,
    defaultModeId,
    restoreModes,
    readMode,
    copyMode,
    type AgentMode,
  } from '../lib/agent-modes';

  let {
    workspace,
    workspaceName,
    files,
    activeFile,
    visible,
    onclose,
    onreadfile,
  }: {
    workspace: string;
    workspaceName: string;
    files: ChatFile[];
    activeFile: ChatFile | null;
    visible: boolean;
    onclose: () => void;
    onreadfile: (path: string) => Promise<string>;
  } = $props();
  let chats = $state<DraftChat[]>([]);
  let modes = $state<AgentMode[]>(defaultModes());
  let managingMode = $state(false);
  let selectedId = $state('');
  let loadedKey = $state('');
  let storageError = $state('');
  let historyVisible = $state(false);
  let pickerVisible = $state(false);
  let query = $state('');
  let composer = $state<HTMLTextAreaElement>();
  let messageList = $state<HTMLDivElement>();
  let historyRoot: HTMLDivElement;
  let contextRoot = $state<HTMLDivElement>();
  let connection = $state<AgentConnection>({
    connected: false,
    error: '',
  });
  let connecting = $state(false);
  let sending = $state(false);
  let loadingChat = $state(false);
  let agentError = $state('');
  let conversations = $state<Record<string, AgentConversation>>({});
  let codexThreads = $state<AgentThread[]>([]);
  let nextThreadCursor = $state<string | null>(null);
  let connectionError = $state('');
  let connectAttempt: Promise<boolean> | undefined;
  let retryAt = 0;
  let disposed = false;
  type PendingPrompt = {
    id: string;
    target: number;
    chatId: string;
    threadId: string;
    key: string;
    text: string;
    mode: AgentMode;
    files: ChatFile[];
  };
  const challengeGate = new PromptChallenge<PendingPrompt>();
  let pendingPrompt = $state<PendingPrompt | null>(null);
  let challengeTarget = $state(SIX_SEVEN_TARGET);
  let challengeTargetError = $state('');
  const chat = $derived(chats.find((item) => item.id === selectedId));
  const conversation = $derived(
    chat?.threadId ? conversations[chat.threadId] : undefined,
  );
  const mode = $derived(
    modes.find((item) => item.id === chat?.modeId) ?? modes[0],
  );
  const filteredFiles = $derived(
    files
      .filter((file) =>
        `${file.name} ${relative(file.path)}`
          .toLowerCase()
          .includes(query.toLowerCase()),
      )
      .slice(0, 50),
  );

  function blankChat(modeId = defaultModeId): DraftChat {
    return {
      id: crypto.randomUUID(),
      title: 'New chat',
      messages: [],
      composer: '',
      files: [],
      modeId,
      threadId: '',
    };
  }
  function references(value: unknown): ChatFile[] {
    if (!Array.isArray(value)) return [];
    return value
      .filter(
        (file) =>
          file &&
          typeof file.name === 'string' &&
          typeof file.path === 'string',
      )
      .map((file) => ({ name: file.name, path: file.path }));
  }
  function restore(key: string) {
    modes = defaultModes();
    try {
      const saved = JSON.parse(localStorage.getItem(key) ?? 'null');
      modes = restoreModes(saved?.modes);
      if (saved && Array.isArray(saved.chats)) {
        const restored: DraftChat[] = saved.chats
          .filter(
            (item: DraftChat) =>
              item &&
              typeof item.id === 'string' &&
              typeof item.title === 'string' &&
              Array.isArray(item.messages),
          )
          .map((item: DraftChat) => ({
            id: item.id,
            title: item.title,
            composer: typeof item.composer === 'string' ? item.composer : '',
            files: references(item.files),
            modeId: modes.some((mode) => mode.id === item.modeId)
              ? item.modeId
              : defaultModeId,
            threadId: typeof item.threadId === 'string' ? item.threadId : '',
            messages: item.messages
              .filter(
                (draft) =>
                  draft &&
                  typeof draft.id === 'string' &&
                  typeof draft.text === 'string' &&
                  typeof draft.createdAt === 'string',
              )
              .map((draft) => ({
                ...draft,
                files: references(draft.files),
                mode: readMode(draft.mode) ?? undefined,
              })),
          }));
        if (restored.length) {
          chats = restored;
          selectedId = restored.some((item) => item.id === saved.selectedId)
            ? saved.selectedId
            : restored[0].id;
          return;
        }
      }
    } catch {}
    const first = blankChat();
    chats = [first];
    selectedId = first.id;
  }
  function persist() {
    if (!loadedKey) return;
    try {
      localStorage.setItem(
        loadedKey,
        JSON.stringify({ chats, selectedId, modes }),
      );
      storageError = '';
    } catch {
      storageError =
        'Your chats and modes could not be saved. Keep this panel open to retain them.';
    }
  }
  $effect(() => {
    const key = 'praewa-agent-drafts:' + (workspace || 'no-workspace');
    if (loadedKey === key) return;
    untrack(() => {
      cancelChallenge();
      persist();
      restore(key);
      loadedKey = key;
      historyVisible = false;
      pickerVisible = false;
      managingMode = false;
      agentError = '';
      codexThreads = [];
      nextThreadCursor = null;
      storageError = '';
    });
  });
  $effect(() => {
    const key = loadedKey;
    const data = JSON.stringify({ chats, selectedId, modes });
    if (!key) return;
    const timer = setTimeout(() => {
      try {
        localStorage.setItem(key, data);
        storageError = '';
      } catch {
        storageError =
          'Your chats and modes could not be saved. Keep this panel open to retain them.';
      }
    }, 200);
    return () => clearTimeout(timer);
  });
  $effect(() => {
    const pending = pendingPrompt;
    if (pending && !matchesPrompt(pending)) untrack(cancelChallenge);
  });
  $effect(() => {
    if (pendingPrompt?.id)
      void tick().then(() => {
        if (messageList) messageList.scrollTop = 0;
      });
  });
  $effect(() => {
    if (visible && !managingMode) void tick().then(() => composer?.focus());
    else {
      historyVisible = false;
      pickerVisible = false;
    }
  });
  $effect(() => {
    const count = (chat?.messages.length ?? 0) + (conversation?.sequence ?? 0);
    if (visible && !managingMode && count)
      void tick().then(() => {
        if (messageList) messageList.scrollTop = messageList.scrollHeight;
      });
  });
  onDestroy(persist);
  onMount(() => {
    try {
      challengeTarget =
        parseSixSevenTarget(localStorage.getItem(SIX_SEVEN_TARGET_STORAGE)) ??
        SIX_SEVEN_TARGET;
    } catch {}
    const lastSnapshots = new Map<string, number>();
    let lastStatusAt = Date.now();
    const off = onAgentEvent((snapshot) => {
      lastSnapshots.set(snapshot.thread.id, Date.now());
      applyConversation(snapshot);
    });
    let polling = false;
    void tick().then(() => connectAgent());
    const timer = setInterval(async () => {
      if (polling || connecting || disposed) return;
      if (!connection.connected) {
        if (Date.now() >= retryAt) void connectAgent();
        return;
      }
      polling = true;
      try {
        const interval = window.runtime?.EventsOn ? 5000 : 700;
        if (
          visible &&
          !pendingPrompt &&
          !managingMode &&
          !document.hidden &&
          chat?.threadId &&
          Date.now() - (lastSnapshots.get(chat.threadId) ?? 0) >= interval
        ) {
          lastSnapshots.set(chat.threadId, Date.now());
          applyConversation(await agentBackend.snapshot(chat.threadId));
        }
      } catch (reason) {
        agentError = reason instanceof Error ? reason.message : String(reason);
      } finally {
        try {
          if (Date.now() - lastStatusAt >= 5000) {
            lastStatusAt = Date.now();
            const next = await agentBackend.status();
            if (!disposed) {
              connection = next;
              if (!next.connected) retryAt = 0;
            }
          }
        } catch {
          if (!disposed) {
            connection.connected = false;
            retryAt = 0;
          }
        }
        polling = false;
      }
    }, 700);
    return () => {
      disposed = true;
      off();
      clearInterval(timer);
    };
  });

  function applyConversation(next: AgentConversation) {
    if (disposed || !next.thread?.id) return;
    const old = conversations[next.thread.id];
    if (!old || next.sequence > old.sequence)
      conversations[next.thread.id] = next;
  }
  function connectAgent(): Promise<boolean> {
    if (disposed) return Promise.resolve(false);
    if (connectAttempt) return connectAttempt;
    connecting = true;
    connectionError = '';
    connectAttempt = (async () => {
      try {
        const next = await agentBackend.connect();
        if (disposed) return false;
        connection = next;
        if (!next.connected) {
          connectionError =
            next.error || 'Your agent is unavailable. Try again.';
          return false;
        }
        const id = chat?.threadId;
        const key = loadedKey;
        if (id) {
          try {
            const loaded = await agentBackend.load(id);
            if (!disposed && key === loadedKey) applyConversation(loaded);
          } catch (reason) {
            if (!disposed && key === loadedKey)
              agentError =
                reason instanceof Error ? reason.message : String(reason);
          }
        }
        if (historyVisible) void loadHistory();
        return true;
      } catch (reason) {
        if (!disposed)
          connectionError =
            reason instanceof Error ? reason.message : String(reason);
        return false;
      } finally {
        connecting = false;
        retryAt = Date.now() + 10000;
        connectAttempt = undefined;
      }
    })();
    return connectAttempt;
  }
  async function loadHistory(more = false) {
    agentError = '';
    const key = loadedKey;
    try {
      const page = await agentBackend.threads(
        more ? (nextThreadCursor ?? '') : '',
      );
      if (disposed || key !== loadedKey) return;
      codexThreads = more ? [...codexThreads, ...page.data] : page.data;
      nextThreadCursor = page.nextCursor;
    } catch (reason) {
      if (!disposed && key === loadedKey)
        agentError = reason instanceof Error ? reason.message : String(reason);
    }
  }
  async function chooseCodexChat(thread: AgentThread) {
    loadingChat = true;
    agentError = '';
    const key = loadedKey;
    try {
      const loaded = await agentBackend.load(thread.id);
      if (loadedKey !== key) return;
      applyConversation(loaded);
      let target = chats.find((item) => item.threadId === thread.id);
      if (!target) {
        target = {
          ...blankChat(mode.id),
          threadId: thread.id,
          title: thread.name || thread.preview.slice(0, 48) || 'Chat',
        };
        chats.push(target);
      }
      selectedId = target.id;
      historyVisible = false;
      managingMode = false;
      focusComposer();
    } catch (reason) {
      agentError = reason instanceof Error ? reason.message : String(reason);
    } finally {
      loadingChat = false;
    }
  }
  function cancelChallenge() {
    challengeGate.cancel();
    pendingPrompt = null;
  }
  function setChallengeTarget(input: HTMLInputElement) {
    const target = parseSixSevenTarget(input.value);
    if (target === null) {
      input.value = String(challengeTarget);
      challengeTargetError = `ใช้จำนวนเต็ม ${SIX_SEVEN_MIN_TARGET}–${SIX_SEVEN_MAX_TARGET.toLocaleString()} รอบ`;
      return;
    }
    challengeTargetError = '';
    input.value = String(target);
    if (target === challengeTarget) return;
    challengeTarget = target;
    try {
      localStorage.setItem(SIX_SEVEN_TARGET_STORAGE, String(target));
    } catch {}
    if (pendingPrompt) {
      const pending = { ...pendingPrompt, id: crypto.randomUUID(), target };
      challengeGate.begin(pending.id, pending, target);
      pendingPrompt = pending;
    }
  }
  function matchesPrompt(pending: PendingPrompt) {
    return (
      !disposed &&
      visible &&
      !document.hidden &&
      !managingMode &&
      !loadingChat &&
      !conversation?.active &&
      loadedKey === pending.key &&
      chat?.id === pending.chatId &&
      chat.threadId === pending.threadId &&
      chat.composer.trim() === pending.text &&
      JSON.stringify(copyMode(mode)) === JSON.stringify(pending.mode) &&
      JSON.stringify(chat.files) === JSON.stringify(pending.files)
    );
  }
  function sendMessage() {
    if (
      !chat?.composer.trim() ||
      !visible ||
      document.hidden ||
      pendingPrompt ||
      sending ||
      loadingChat ||
      conversation?.active
    )
      return;
    const pending: PendingPrompt = {
      id: crypto.randomUUID(),
      target: challengeTarget,
      chatId: chat.id,
      threadId: chat.threadId,
      key: loadedKey,
      text: chat.composer.trim(),
      mode: copyMode(mode),
      files: chat.files.map((file) => ({ ...file })),
    };
    challengeGate.begin(pending.id, pending, pending.target);
    pendingPrompt = pending;
    historyVisible = false;
    pickerVisible = false;
    agentError = '';
  }
  function completeChallenge(id: string, count: number) {
    if (!pendingPrompt || pendingPrompt.id !== id) return;
    if (!matchesPrompt(pendingPrompt)) {
      cancelChallenge();
      return;
    }
    const approved = challengeGate.complete(id, count);
    if (!approved) return;
    pendingPrompt = null;
    void sendApprovedMessage(approved);
  }
  async function sendApprovedMessage(pending: PendingPrompt) {
    if (sending || !matchesPrompt(pending)) return;
    const target = chat!;
    const { text, mode: selectedMode, files } = pending;
    sending = true;
    agentError = '';
    try {
      if (!connection.connected && !(await connectAgent())) return;
      if (!matchesPrompt(pending)) return;
      const context = await Promise.all(
        files.map(async (file) => ({
          ...file,
          content: await onreadfile(file.path),
        })),
      );
      if (!matchesPrompt(pending)) return;
      const result = await agentBackend.send({
        id: pending.id,
        threadId: pending.threadId,
        text,
        mode: selectedMode,
        files: context,
      });
      if (result.conversation.thread?.id) {
        target.threadId = result.conversation.thread.id;
        applyConversation(result.conversation);
      }
      if (result.error) throw new Error(result.error);
      if (target.title === 'New chat')
        target.title = text.replace(/\s+/g, ' ').slice(0, 48);
      if (target.composer.trim() === text) target.composer = '';
      target.files = target.files.filter(
        (file) => !files.some((sent) => sent.path === file.path),
      );
      pickerVisible = false;
      focusComposer();
    } catch (reason) {
      agentError = reason instanceof Error ? reason.message : String(reason);
    } finally {
      sending = false;
    }
  }
  async function stopAgent() {
    if (!chat?.threadId) return;
    try {
      await agentBackend.stop(chat.threadId);
    } catch (reason) {
      agentError = reason instanceof Error ? reason.message : String(reason);
    }
  }

  function relative(path: string) {
    if (path.startsWith('untitled:')) return 'Unsaved editor buffer';
    return workspace && path.startsWith(workspace + '/')
      ? path.slice(workspace.length + 1)
      : path;
  }
  function time(value: string) {
    const date = new Date(value);
    return Number.isNaN(date.getTime())
      ? ''
      : date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  }
  function focusComposer() {
    void tick().then(() => composer?.focus());
  }
  function newChat() {
    cancelChallenge();
    historyVisible = false;
    managingMode = false;
    if (
      chat &&
      !chat.messages.length &&
      !chat.composer.trim() &&
      !chat.files.length &&
      !chat.threadId
    ) {
      focusComposer();
      return;
    }
    const next = blankChat(mode.id);
    chats = [...chats, next];
    selectedId = next.id;
    focusComposer();
  }
  function attach(file: ChatFile) {
    if (pendingPrompt || sending) return;
    if (chat && !chat.files.some((item) => item.path === file.path))
      chat.files = [...chat.files, { ...file }];
    pickerVisible = false;
    focusComposer();
  }
  function suggest(text: string, includeFile = false) {
    if (!chat || pendingPrompt || sending) return;
    chat.composer = text;
    if (includeFile && activeFile) attach(activeFile);
    else focusComposer();
  }
  function outside(event: Event) {
    if (!(event.target instanceof Node)) return;
    if (!historyRoot?.contains(event.target)) historyVisible = false;
    if (!contextRoot?.contains(event.target)) pickerVisible = false;
  }
</script>

<svelte:window
  onpointerdown={outside}
  onbeforeunload={() => {
    cancelChallenge();
    persist();
  }}
  onkeydown={(event) => {
    if (event.key === 'Escape' && pendingPrompt) {
      event.preventDefault();
      cancelChallenge();
      focusComposer();
    } else if (event.key === 'Escape' && (historyVisible || pickerVisible)) {
      event.preventDefault();
      historyVisible = false;
      pickerVisible = false;
      focusComposer();
    }
  }}
/>

<section class="agent-panel" aria-label="Agent conversation">
  <header class="agent-heading">
    <span class="heading-label"><Bot size={17} /><span>Agent</span></span>
    <div class="chat-actions" bind:this={historyRoot}>
      <button
        class="icon-button"
        aria-label="New agent chat"
        title="New chat"
        onclick={newChat}><Plus size={15} /></button
      >
      <button
        class="icon-button"
        class:active={historyVisible}
        aria-label="Chat history"
        aria-expanded={historyVisible}
        title="Chat history"
        onclick={() => {
          historyVisible = !historyVisible;
          if (historyVisible && connection.connected) void loadHistory();
        }}><History size={15} /></button
      >
      <button
        class="icon-button"
        aria-label="Close agent chat"
        title="Close"
        onclick={() => {
          cancelChallenge();
          onclose();
        }}><X size={15} /></button
      >
      {#if historyVisible}<div class="chat-history" aria-label="Saved chats">
          <div class="picker-label">CHAT HISTORY</div>
          {#each [...chats].reverse() as item}<button
              class:current={item.id === selectedId}
              onclick={() => {
                cancelChallenge();
                selectedId = item.id;
                historyVisible = false;
                managingMode = false;
                focusComposer();
              }}
              ><span>{item.title}</span><small
                >{item.threadId
                  ? 'Chat'
                  : `${item.messages.length} local drafts`}</small
              ></button
            >{/each}
          {#each codexThreads.filter((thread) => !chats.some((saved) => saved.threadId === thread.id)) as thread}
            <button
              disabled={loadingChat}
              onclick={() => chooseCodexChat(thread)}
            >
              <span
                >{thread.name ||
                  thread.preview.slice(0, 70) ||
                  'Untitled chat'}</span
              >
              <small
                >{new Date(thread.updatedAt * 1000).toLocaleDateString()}</small
              >
            </button>
          {/each}
          {#if nextThreadCursor}<button onclick={() => loadHistory(true)}
              >Load more chats</button
            >{/if}
        </div>{/if}
    </div>
  </header>
  <div
    class="chat-workspace"
    title={workspace || 'Open a workspace to add files'}
  >
    <FolderOpen size={12} /><span>{workspaceName || 'No workspace'}</span>
  </div>
  {#if chat}
    <fieldset
      class="m-0 min-w-0 border-0 p-0"
      disabled={!!pendingPrompt || sending}
    >
      <AgentModes
        {workspace}
        {files}
        {modes}
        selected={mode}
        {visible}
        bind:editing={managingMode}
        {onreadfile}
        onselect={(id) => {
          if (chat) chat.modeId = id;
        }}
        onsave={(saved) => {
          modes = modes.some((item) => item.id === saved.id)
            ? modes.map((item) => (item.id === saved.id ? saved : item))
            : [...modes, saved];
          if (chat) chat.modeId = saved.id;
        }}
      />
    </fieldset>
    {#if !managingMode}
      <div class="chat-messages" bind:this={messageList}>
        {#if pendingPrompt}
          {@const pending = pendingPrompt}
          {#key pending.id}
            <SixSevenChallenge
              target={pending.target}
              oncomplete={(count) => completeChallenge(pending.id, count)}
              oncancel={cancelChallenge}
            />
          {/key}
        {:else}
          {#if loadingChat}<p class="loading-chat">Loading chat…</p>{/if}
          {#if conversation}
            <div class="conversation-title">
              {conversation.thread.name || chat.title}
            </div>
            {#each conversation.messages as message (message.id)}<article
                class="chat-draft"
                class:agent-reply={message.role === 'assistant'}
                aria-label={message.role === 'assistant'
                  ? 'Agent reply'
                  : 'Sent message'}
              >
                <div class="message-meta">
                  <span class="user-avatar"
                    >{#if message.role === 'assistant'}<Bot
                        size={12}
                      />{:else}Y{/if}</span
                  ><strong
                    >{message.role === 'assistant' ? 'Agent' : 'You'}</strong
                  ><span class="message-time"
                    >{message.createdAt
                      ? time(new Date(message.createdAt * 1000).toISOString())
                      : ''}</span
                  >{#if message.status === 'inProgress'}<span
                      class="message-state">Working</span
                    >{/if}
                </div>
                {#if message.role === 'assistant' && message.text}<div
                    class="reply-markdown"
                  >
                    <MarkdownPreview
                      content={message.text}
                      streaming={message.status === 'inProgress'}
                    />
                  </div>{:else}<div class="message-text">
                    {message.text || 'Thinking…'}
                  </div>{/if}
              </article>{/each}
            <AgentRequests
              requests={conversation.requests}
              onerror={(text) => (agentError = text)}
            />
            {#if conversation.active}<div class="agent-working">
                <Circle size={9} />Agent is working…
              </div>{/if}
            {#if conversation.error}<p class="agent-error" role="alert">
                {conversation.error}
              </p>{/if}
          {/if}
          {#if !chat.messages.length && !conversation}<div class="chat-welcome">
              <div class="agent-symbol">
                <Bot size={28} /><span><Sparkles size={10} /></span>
              </div>
              <h2>
                {mode.behavior === 'chat'
                  ? `Talk to ${mode.name}`
                  : 'What are we building?'}
              </h2>
              <p>
                {mode.description}
              </p>
              <div class="chat-suggestions">
                <button
                  onclick={() =>
                    suggest(
                      'Explain this file and how it fits into the project.',
                      true,
                    )}
                  ><FileCode2 size={14} /><span>Explain this file</span></button
                >
                <button
                  onclick={() =>
                    suggest('Help me plan the changes I want to make.')}
                  ><PencilRuler size={14} /><span>Plan a change</span></button
                >
                <button
                  onclick={() =>
                    suggest(
                      'Help me find and understand a bug in this file.',
                      true,
                    )}><Bug size={14} /><span>Find a bug</span></button
                >
              </div>
            </div>
          {:else if chat.messages.length}
            <div class="conversation-title">
              {conversation ? 'LOCAL DRAFTS' : chat.title}
            </div>
            {#each chat.messages as draft (draft.id)}<article
                class="chat-draft"
                aria-label="Saved message draft"
              >
                <div class="message-meta">
                  <span class="user-avatar">Y</span><strong>You</strong><span
                    class="message-time">{time(draft.createdAt)}</span
                  ><span class="message-state">Draft</span>
                </div>
                {#if draft.mode}<div
                    class="message-mode"
                    title={draft.mode.description}
                  >
                    <Bot size={11} /><span>{draft.mode.name}</span>
                    {#if draft.mode.skills.length}<span
                        >· {draft.mode.skills.length} skill{draft.mode.skills
                          .length === 1
                          ? ''
                          : 's'}</span
                      >{/if}
                  </div>{/if}
                <div class="message-text">{draft.text}</div>
                {#if connection.connected}<button
                    class="use-draft"
                    onclick={() => {
                      if (chat) {
                        chat.composer = draft.text;
                        chat.files = draft.files.map((file) => ({ ...file }));
                        focusComposer();
                      }
                    }}>Use this draft</button
                  >{/if}
                {#if draft.files.length}<div class="message-files">
                    {#each draft.files as file}<span title={relative(file.path)}
                        ><FileIcon
                          name={file.name}
                          size={13}
                        />{file.name}</span
                      >{/each}
                  </div>{/if}
              </article>{/each}
          {/if}
        {/if}
      </div>
      {#if connectionError}<div class="agent-connection-error" role="alert">
          <p>{connectionError}</p>
          <button disabled={connecting} onclick={() => void connectAgent()}
            ><RefreshCw size={12} />Try again</button
          >
        </div>{/if}
      {#if agentError}<p class="agent-error" role="alert">{agentError}</p>{/if}
      {#if storageError}<p class="draft-storage-error" role="alert">
          {storageError}
        </p>{/if}
      <div class="composer-area" bind:this={contextRoot}>
        <div
          class="mb-2 flex flex-wrap items-center justify-between gap-2 text-[11px] text-muted"
        >
          <label class="flex items-center gap-2">
            <span>Six Seven</span>
            <input
              type="number"
              min={SIX_SEVEN_MIN_TARGET}
              max={SIX_SEVEN_MAX_TARGET}
              step="1"
              value={challengeTarget}
              aria-label="จำนวนรอบ Six Seven"
              aria-invalid={!!challengeTargetError}
              class="w-[72px] rounded-md border border-line bg-surface px-2 py-1 font-mono text-accent outline-none focus:border-accent"
              disabled={sending || loadingChat || !!conversation?.active}
              onchange={(event) => setChallengeTarget(event.currentTarget)}
              onkeydown={(event) => {
                if (event.key === 'Enter') {
                  event.preventDefault();
                  event.stopPropagation();
                  setChallengeTarget(event.currentTarget);
                  event.currentTarget.blur();
                }
              }}
            />
            <span>รอบ</span>
          </label>
          <span
            >{SIX_SEVEN_MIN_TARGET}–{SIX_SEVEN_MAX_TARGET.toLocaleString()} รอบ</span
          >
        </div>
        {#if challengeTargetError}
          <p class="mb-2 text-[11px] text-red-300" role="alert">
            {challengeTargetError}
          </p>
        {/if}
        {#if pendingPrompt}
          <p class="mb-2 text-[11px] text-muted">
            เปลี่ยนจำนวนแล้วจะเริ่มนับใหม่
          </p>
        {/if}
        <fieldset
          class="m-0 min-w-0 border-0 p-0"
          disabled={!!pendingPrompt || sending}
        >
          <div class="composer-context">
            <button
              class="context-button"
              class:active={pickerVisible}
              aria-label="Add file context"
              aria-expanded={pickerVisible}
              disabled={!files.length}
              title="Add file context"
              onclick={() => {
                query = '';
                pickerVisible = !pickerVisible;
              }}><Paperclip size={13} /><span>Add context</span></button
            >
            {#if activeFile && !chat.files.some((file) => file.path === activeFile.path)}<button
                class="active-file-button"
                title={`Add ${relative(activeFile.path)}`}
                aria-label={`Attach active file ${activeFile.name}`}
                onclick={() => attach(activeFile)}
                ><FileIcon name={activeFile.name} size={13} /><span
                  >{activeFile.name}</span
                ><Plus size={10} /></button
              >{/if}
          </div>
          {#if pickerVisible}<div
              class="context-picker"
              aria-label="Context files"
            >
              <div class="context-search">
                <Search size={13} /><input
                  aria-label="Filter context files"
                  placeholder="Find a file…"
                  bind:value={query}
                />
              </div>
              <div class="context-files">
                {#each filteredFiles as file}<button
                    title={relative(file.path)}
                    class:attached={chat.files.some(
                      (item) => item.path === file.path,
                    )}
                    onclick={() => attach(file)}
                    ><FileIcon name={file.name} /><span
                      >{file.name}<small>{relative(file.path)}</small></span
                    ><Plus size={12} /></button
                  >{/each}{#if !filteredFiles.length}<p>
                    No matching files
                  </p>{/if}
              </div>
            </div>{/if}
          <div class="message-composer">
            {#if chat.files.length}<div class="attached-files">
                {#each chat.files as file}<span
                    class="context-chip"
                    title={relative(file.path)}
                    ><FileIcon name={file.name} size={13} /><span
                      >{file.name}</span
                    ><button
                      aria-label={`Remove ${file.name} from context`}
                      onclick={() => {
                        if (chat)
                          chat.files = chat.files.filter(
                            (item) => item.path !== file.path,
                          );
                      }}><X size={11} /></button
                    ></span
                  >{/each}
              </div>{/if}
            <textarea
              bind:this={composer}
              bind:value={chat.composer}
              aria-label="Message for agent"
              placeholder="Write a message for your agent…"
              maxlength="20000"
              onkeydown={(event) => {
                if (
                  event.key === 'Enter' &&
                  !event.shiftKey &&
                  !event.isComposing
                ) {
                  event.preventDefault();
                  event.stopPropagation();
                  void sendMessage();
                }
              }}></textarea>
            <div class="composer-toolbar">
              {#if conversation?.canStop}<button
                  class="save-draft stop-agent"
                  aria-label="Stop agent response"
                  onclick={stopAgent}
                  ><Square size={12} /><span>Stop</span></button
                >
              {:else}<button
                  class="save-draft"
                  disabled={!chat.composer.trim() ||
                    sending ||
                    loadingChat ||
                    !!conversation?.active}
                  aria-label="Send message to agent"
                  title="Send message"
                  onclick={() => void sendMessage()}
                  ><span>{sending ? 'Sending…' : 'Send'}</span><ArrowUp
                    size={15}
                  /></button
                >{/if}
            </div>
          </div>
        </fieldset>
        <div class="composer-hint">
          {#if pendingPrompt}ทำ Six Seven ครบ {pendingPrompt.target} รอบเพื่อส่งข้อความ{:else}<kbd
              >Enter</kbd
            >
            → Six Seven {challengeTarget} รอบ ·
            <kbd>Shift + Enter</kbd> for a new line
          {/if}
        </div>
      </div>
    {/if}
  {/if}
</section>

<style lang="postcss">
  @reference '../styles.css';

  .reply-markdown :global(.markdown-preview) {
    @apply h-auto overflow-visible p-0 text-[12px] leading-[1.7] wrap-anywhere;
  }
  .reply-markdown :global(.markdown-preview > :first-child) {
    @apply mt-0;
  }
  .reply-markdown :global(.markdown-preview > :last-child) {
    @apply mb-0;
  }
  .reply-markdown :global(pre) {
    @apply p-[11px] leading-[1.6];
  }
  .reply-markdown :global(pre code) {
    @apply text-[10px];
  }
  .agent-connection-error {
    @apply py-[10px] px-[13px] text-[#e3a18d] text-[10px] leading-[1.6];
  }
  .agent-connection-error p {
    @apply mt-0 mr-0 mb-[7px] ml-0 wrap-anywhere;
  }
  .agent-connection-error button {
    @apply flex items-center gap-[5px] text-[#e0cc85] text-[10px] py-[5px] px-0;
  }
  .agent-error {
    @apply m-0 py-[10px] px-[13px] text-[#e3a18d] text-[10px] leading-[1.6] wrap-anywhere;
  }
  .loading-chat {
    @apply text-[#949dac] text-[11px];
  }
  .agent-reply .message-text {
    @apply border-0 bg-transparent p-0;
  }
  .agent-reply .user-avatar {
    @apply text-[#dfcf84] bg-[#f5d54715];
  }
  .agent-working {
    @apply flex items-center gap-[6px] text-[#b3a66c] text-[10px] py-[10px] px-0;
  }
  .agent-working :global(svg) {
    animation: blink 1.4s ease infinite;
  }
  @keyframes blink {
    50% {
      opacity: 0.3;
    }
  }
  .use-draft {
    @apply text-[9px] text-[#b5a870] py-[5px] px-0;
  }
  .stop-agent {
    @apply bg-[#363323] text-[#edd580];
  }
  .agent-panel {
    @apply h-full min-h-0 flex flex-col bg-surface;
  }
  .agent-heading {
    @apply flex items-center justify-between h-[45px] py-0 px-[13px] border-b border-b-line shrink-0 bg-[#131417];
  }
  .heading-label {
    @apply flex items-center gap-[8px] text-[12px] font-semibold;
  }
  .heading-label :global(svg) {
    @apply text-accent;
  }
  .chat-actions {
    @apply flex gap-[3px] relative;
  }
  .chat-actions .icon-button {
    @apply w-[26px] h-[26px];
  }
  .chat-actions .active {
    @apply text-accent bg-[#f5d54712];
  }
  .chat-workspace {
    @apply flex items-center gap-[6px] h-[34px] py-0 px-[13px] text-[#818794] text-[10px] shrink-0;
  }
  .chat-workspace > span:first-of-type {
    @apply overflow-hidden text-ellipsis whitespace-nowrap flex-1;
  }
  .chat-messages {
    @apply flex-1 min-h-0 overflow-y-auto p-[14px];
    scrollbar-width: thin;
    scrollbar-color: #37383e transparent;
  }
  .chat-welcome {
    @apply pt-[clamp(12px,_4vh,_55px)] text-center;
  }
  .agent-symbol {
    @apply relative inline-flex items-center justify-center h-[54px] w-[54px];
    background: linear-gradient(140deg, #f5d54714, #f5d54704);
    @apply border border-[#f5d54736] rounded-[15px] text-accent;
  }
  .agent-symbol > span {
    @apply absolute flex right-[-4px] top-[-4px] bg-accent text-surface border-3 border-surface p-[3px] rounded-[50%];
  }
  h2 {
    @apply text-[17px] tracking-[-0.4px] mt-[18px] mr-0 mb-[9px] ml-0 font-[550];
  }
  .chat-welcome > p {
    @apply m-0 text-[11px] leading-[1.8] text-[#818794];
  }
  .chat-suggestions {
    @apply grid gap-[7px] mt-[23px];
  }
  .chat-suggestions button {
    @apply flex items-center gap-[10px] w-full border border-[#2b2d33] bg-[#18191d] py-[11px] px-[12px] rounded-[7px] text-left text-[11px] text-[#b3b7c1];
  }
  .chat-suggestions button :global(svg) {
    @apply text-[#9d946c];
  }
  .chat-suggestions button:hover {
    @apply bg-[#f5d54709] border-[#5d5330] text-accent;
  }
  .composer-area {
    @apply pt-[10px] pr-[13px] pb-[9px] pl-[13px] shrink-0 relative;
  }
  .composer-context {
    @apply flex items-center gap-[8px] mb-[8px] min-w-0;
  }
  .context-button,
  .active-file-button {
    @apply flex items-center gap-[5px] text-[#858b98] py-[3px] px-0 text-[10px];
  }
  .context-button {
    @apply shrink-0;
  }
  .context-button:hover,
  .context-button.active,
  .active-file-button:hover {
    @apply text-accent;
  }
  .active-file-button {
    @apply min-w-0 ml-auto max-w-[50%];
  }
  .active-file-button span {
    @apply whitespace-nowrap overflow-hidden text-ellipsis;
  }
  .message-composer {
    @apply border border-[#363832] rounded-[8px] bg-[#191a1e] overflow-hidden;
  }
  .message-composer:focus-within {
    @apply border-[#b7a24e];
    box-shadow: 0 0 0 1px #f5d54708;
  }
  textarea {
    @apply block w-full h-[94px] min-h-[60px] resize-none border-0;
    outline: none;
    @apply p-[12px];
    font: inherit;
    @apply text-[12px] leading-[1.65] text-[#d8dae1] bg-transparent;
  }
  textarea::placeholder {
    @apply text-[#6b707c];
  }
  .composer-toolbar {
    @apply flex items-center justify-end gap-[6px] pt-0 pr-[8px] pb-[8px] pl-[8px];
  }
  .save-draft {
    @apply flex items-center gap-[5px] bg-accent text-[#191910] py-[6px] px-[8px] rounded-[5px] text-[10px] font-semibold whitespace-nowrap;
  }
  .save-draft:hover:not(:disabled) {
    @apply bg-[#ffe576];
  }
  .composer-hint {
    @apply pt-[8px] text-center text-[9px] text-[#646a75];
  }
  .composer-hint kbd {
    @apply text-[9px] text-[#858b98];
  }
  .attached-files,
  .message-files {
    @apply flex gap-[5px] flex-wrap;
  }
  .attached-files {
    @apply pt-[9px] pr-[9px] pb-0 pl-[9px];
  }
  .context-chip {
    @apply flex items-center gap-[5px] bg-[#24262b] border border-[#393b43] rounded-[4px] pt-[3px] pr-[4px] pb-[3px] pl-[6px] text-[10px] min-w-0 max-w-full;
  }
  .context-chip > span {
    @apply overflow-hidden text-ellipsis whitespace-nowrap;
  }
  .context-chip button {
    @apply p-[2px] flex rounded-[2px] text-[#868d99];
  }
  .context-chip button:hover {
    @apply text-accent bg-[#f5d54712];
  }
  .conversation-title {
    @apply text-[#7b818d] text-[10px] pb-[18px] wrap-anywhere;
  }
  .chat-draft {
    @apply mb-[16px];
  }
  .message-meta {
    @apply flex items-center gap-[7px] text-[10px] mb-[8px];
  }
  .user-avatar {
    @apply flex items-center justify-center w-[20px] h-[20px] rounded-[5px] bg-[#313029] text-[#e1ce76] text-[9px];
  }
  .message-meta strong {
    @apply font-medium text-[#bdc1ca];
  }
  .message-time {
    @apply text-[#666d79] text-[9px] ml-auto;
  }
  .message-state {
    @apply border border-[#45402b] text-[#b4a66b] text-[8px] py-[2px] px-[5px] rounded-[3px];
  }
  .message-text {
    @apply text-[12px] leading-[1.75] whitespace-pre-wrap wrap-anywhere bg-[#1a1b1f] border border-[#2c2e35] rounded-[7px] py-[11px] px-[12px];
  }
  .message-mode {
    @apply flex items-center gap-[5px] text-[9px] text-[#b5a870] mt-[-1px] mr-0 mb-[8px] ml-0 wrap-anywhere;
  }
  .message-files {
    @apply mt-[7px];
  }
  .message-files > span {
    @apply flex items-center gap-[5px] text-[#959caa] text-[9px] bg-[#202228] py-[4px] px-[6px] rounded-[4px] wrap-anywhere min-w-0;
  }
  .chat-history,
  .context-picker {
    @apply absolute z-[25] bg-[#191a1e] border border-[#3d3b30] rounded-[6px];
    box-shadow: 0 8px 24px #0008;
    @apply overflow-hidden;
  }
  .chat-history {
    @apply top-[32px] right-0 w-[260px] max-h-[260px] overflow-y-auto p-[5px];
  }
  .picker-label {
    @apply text-[#727987] text-[9px] py-[7px] px-[8px] tracking-[0.6px];
  }
  .chat-history button {
    @apply block w-full text-left py-[9px] px-[8px] text-[11px] rounded-[4px];
  }
  .chat-history button span {
    @apply block overflow-hidden text-ellipsis whitespace-nowrap;
  }
  .chat-history button small {
    @apply block text-[9px] text-[#7b8391] mt-[4px];
  }
  .chat-history button:hover,
  .chat-history button.current {
    @apply bg-[#f5d5470d] text-[#e4cf81];
  }
  .context-picker {
    @apply left-[13px] right-[13px] bottom-[calc(100%_-_34px)];
  }
  .context-search {
    @apply flex items-center gap-[7px] text-[#818794] p-[9px] border-b border-b-[#2e3037];
  }
  .context-search input {
    @apply w-full bg-transparent text-[#d8dae1] border-0;
    outline: none;
    @apply text-[11px];
  }
  .context-files {
    @apply max-h-[210px] overflow-y-auto p-[4px];
  }
  .context-files button {
    @apply flex items-center gap-[8px] w-full p-[7px] text-[11px] text-left rounded-[4px];
  }
  .context-files button:hover,
  .context-files button.attached {
    @apply bg-[#f5d5470d];
  }
  .context-files button > span {
    @apply flex-1 min-w-0;
  }
  .context-files small {
    @apply block overflow-hidden text-ellipsis whitespace-nowrap text-[9px] text-[#7c8492] mt-[3px];
  }
  .context-files > p {
    @apply text-[#8a919f] text-[11px] p-[8px];
  }
  .draft-storage-error {
    @apply text-[#d99b96] text-[10px] mt-[8px] mr-[14px] mb-0 ml-[14px];
  }
  @media (max-height: 780px) {
    .chat-welcome {
      @apply pt-[8px];
    }
    .agent-symbol {
      @apply h-[42px] w-[42px] rounded-[12px];
    }
    h2 {
      @apply mt-[12px] text-[16px];
    }
    .chat-suggestions {
      @apply mt-[14px] gap-[5px];
    }
    .chat-suggestions button {
      @apply py-[9px] px-[11px];
    }
    textarea {
      @apply h-[76px];
    }
  }
</style>
