<script lang="ts">
  import { onMount, tick, untrack } from 'svelte';
  import {
    Code2,
    Files,
    FileCode2,
    Search,
    Settings2,
    Terminal,
    FolderOpen,
    FilePlus2,
    FolderPlus,
    ChevronRight,
    ChevronDown,
    Hammer,
    Square,
    X,
    RefreshCw,
    Save,
    Command,
    Check,
    CircleAlert,
    PanelBottom,
    Plus,
    GitBranch,
    Columns2,
    Play,
    Eye,
    Undo2,
    Pencil,
    Trash2,
    ListTree,
    WrapText,
    Minus,
    PanelLeftClose,
    Copy,
    MessageSquare,
  } from '@lucide/svelte';
  import {
    applyEdits,
    type LanguageStatus,
    type DiagnosticsEvent,
  } from './lib/language';
  import Editor from './components/Editor.svelte';
  import FileTree, { type ExplorerEdit } from './components/FileTree.svelte';
  import FileIcon from './components/FileIcon.svelte';
  import ContextMenu from './components/ContextMenu.svelte';
  import TerminalView from './components/TerminalView.svelte';
  import SearchPanel from './components/SearchPanel.svelte';
  import GitPanel from './components/GitPanel.svelte';
  import DiffView from './components/DiffView.svelte';
  import MarkdownPreview from './components/MarkdownPreview.svelte';
  import AgentPanel, { type ChatFile } from './components/AgentPanel.svelte';
  import MenuBar, {
    type MenuDefinition,
    type MenuItem,
  } from './components/MenuBar.svelte';
  import { backend, isDesktop, onBuildOutput, onEvent } from './lib/bridge';
  import {
    language,
    starter,
    type Project,
    type Tab,
    type Entry,
    type Document,
    type Task,
    type TerminalInfo,
    type ShellProfile,
    type GitState,
    type SearchQuery,
    type SearchMatch,
    type TrashedFile,
  } from './lib/types';

  let project = $state<Project | null>(null);
  let tabs = $state<Tab[]>([]);
  let activeId = $state('');
  let splitId = $state('');
  let desktop = $state(false);
  let ready = $state(false);
  let working = $state(false);
  let building = $state(false);
  let activity = $state<'explorer' | 'search' | 'git' | 'tasks'>('explorer');
  let sidebarVisible = $state(true);
  let sidebarWidth = $state(250);
  let panelVisible = $state(true);
  let panelHeight = $state(230);
  let agentVisible = $state(true);
  let agentWidth = $state(340);
  let panel = $state<'terminal' | 'output' | 'problems'>('terminal');
  let output = $state('');
  let terminals = $state<TerminalInfo[]>([]);
  let terminalId = $state('');
  let shells = $state<ShellProfile[]>([]);
  let shell = $state('');
  let tasks = $state<Task[]>([]);
  let git = $state<GitState | null>(null);
  let index = $state<Entry[]>([]);
  let recent = $state<string[]>([]);
  let revision = $state(0);
  let fontSize = $state(14);
  let terminalFontSize = $state(13);
  let wrap = $state(false);
  let tabSize = $state(4);
  let insertSpaces = $state(false);
  let markdown = $state(false);
  let languageStatuses = $state<Record<string, LanguageStatus>>({});
  const languageStatus = $derived(languageStatuses[activeId] ?? null);
  let diagnostics = $state<Record<string, DiagnosticsEvent>>({});
  let cursor = $state({ line: 1, column: 1 });
  let notice = $state('');
  let noticeTimer: ReturnType<typeof setTimeout>;
  let undoTrash = $state<TrashedFile | null>(null);
  let modal = $state<
    | 'commands'
    | 'quick'
    | 'saveas'
    | 'confirm'
    | 'settings'
    | 'line'
    | 'task'
    | 'command'
    | null
  >(null);
  let input = $state('');
  let modalError = $state('');
  let selection = $state(0);
  let confirmText = $state('');
  let pendingAction: (() => void | Promise<void>) | null = null;
  let fileTarget = $state<Entry | null>(null);
  let selectedEntry = $state<Entry | null>(null);
  let explorerEdit = $state<ExplorerEdit | null>(null);
  let explorerMenu = $state<{ entry: Entry; x: number; y: number } | null>(
    null,
  );
  let saveTarget: Tab | null = null;
  let newParent = $state('');
  let consoleElement = $state<HTMLPreElement>();
  let restoreDrafts = $state<Tab[]>([]);
  type EditorHandle = {
    search(): void;
    focus(): void;
    goTo(line: number, column?: number): void;
    undo(): void;
    redo(): void;
  };
  const editors: Record<string, EditorHandle> = {};
  const active = $derived(tabs.find((tab) => tab.id === activeId));
  const split = $derived(tabs.find((tab) => tab.id === splitId));
  const chatFiles = $derived<ChatFile[]>([
    ...new Map(
      [
        ...index,
        ...tabs
          .filter((tab) => !tab.diff && !tab.external)
          .map((tab) => ({
            name: tab.name,
            path: tab.path || `untitled:${tab.id}`,
          })),
      ].map((file) => [file.path, file]),
    ).values(),
  ]);
  const dirty = $derived(tabs.some(needsSave));
  async function readChatFile(path: string) {
    const tab = tabs.find(
      (item) =>
        (item.path || `untitled:${item.id}`) === path &&
        !item.diff &&
        !item.external,
    );
    if (tab) return tab.content;
    return (await backend().ReadFile(path)).content;
  }
  const running = $derived(
    terminals.filter((terminal) => terminal.task && !terminal.exited).length,
  );
  const outline = $derived(
    active && !active.diff
      ? active.content
          .split('\n')
          .flatMap((text, i) => {
            const match = text.match(
              /^\s*(?:export\s+)?(?:async\s+)?(?:func\s+(?:\([^)]*\)\s*)?|function\s+|class\s+|def\s+|fn\s+|(?:const|let)\s+)([\w$]+)/,
            );
            return match ? [{ name: match[1], line: i + 1 }] : [];
          })
          .slice(0, 100)
      : [],
  );
  const commands = [
    {
      label: 'File: Open folder',
      key: 'Ctrl O',
      icon: FolderOpen,
      action: () => openFolder(),
    },
    {
      label: 'File: Open file',
      key: '',
      icon: FileCode2,
      action: () => openDialog(),
    },
    {
      label: 'File: New file',
      key: 'Ctrl N',
      icon: FilePlus2,
      action: () => newFile(),
    },
    {
      label: 'File: New folder',
      key: '',
      icon: FolderPlus,
      action: () => newFolder(),
    },
    {
      label: 'File: Save',
      key: 'Ctrl S',
      icon: Save,
      action: () => saveActive(),
    },
    {
      label: 'File: Save all',
      key: 'Ctrl Shift S',
      icon: Save,
      action: () => saveAll(),
    },
    { label: 'File: Save as', key: '', icon: Copy, action: () => saveAs() },
    {
      label: 'Edit: Undo',
      key: 'Ctrl Z',
      icon: Undo2,
      action: () => {
        if (!active) return;
        editors[active.id]?.undo();
        editors[active.id]?.focus();
      },
    },
    {
      label: 'Edit: Redo',
      key: 'Ctrl Shift Z',
      icon: RefreshCw,
      action: () => {
        if (!active) return;
        editors[active.id]?.redo();
        editors[active.id]?.focus();
      },
    },
    {
      label: 'Edit: Find and replace in file',
      key: 'Ctrl F',
      icon: Search,
      action: () => active && editors[active.id]?.search(),
    },
    {
      label: 'Edit: Format document',
      key: 'Shift Alt F',
      icon: Code2,
      action: () => format(),
    },
    {
      label: 'Search: Find in project',
      key: 'Ctrl Shift F',
      icon: Search,
      action: () => showActivity('search'),
    },
    {
      label: 'Go: Open file quickly',
      key: 'Ctrl P',
      icon: FileCode2,
      action: () => showModal('quick'),
    },
    {
      label: 'Go: Go to line',
      key: 'Ctrl G',
      icon: ListTree,
      action: () => showModal('line'),
    },
    {
      label: 'View: Split editor',
      key: 'Ctrl \\',
      icon: Columns2,
      action: () => toggleSplit(),
    },
    {
      label: 'View: Toggle sidebar',
      key: 'Ctrl B',
      icon: PanelLeftClose,
      action: () => (sidebarVisible = !sidebarVisible),
    },
    {
      label: 'View: Toggle bottom panel',
      key: 'Ctrl J',
      icon: PanelBottom,
      action: () => (panelVisible = !panelVisible),
    },
    {
      label: 'View: Toggle word wrap',
      key: 'Alt Z',
      icon: WrapText,
      action: () => (wrap = !wrap),
    },
    {
      label: 'View: Toggle agent chat',
      key: 'Ctrl Alt A',
      icon: MessageSquare,
      action: () => (agentVisible = !agentVisible),
    },
    {
      label: 'View: Markdown preview',
      key: '',
      icon: Eye,
      action: () => (markdown = !markdown),
    },
    {
      label: 'Source control: Open Git',
      key: '',
      icon: GitBranch,
      action: () => showActivity('git'),
    },
    {
      label: 'Tasks: Run build task',
      key: 'Ctrl Shift B',
      icon: Hammer,
      action: () => compile(),
    },
    {
      label: 'Tasks: Run task',
      key: '',
      icon: Play,
      action: () => showModal('task'),
    },
    {
      label: 'Tasks: Run active file',
      key: 'Ctrl Enter',
      icon: Play,
      action: () => runActive(),
    },
    {
      label: 'Tasks: Configure tasks.json',
      key: '',
      icon: Settings2,
      action: () => configureTasks(),
    },
    {
      label: 'Terminal: New terminal',
      key: 'Ctrl Shift `',
      icon: Plus,
      action: () => newTerminal(),
    },
    {
      label: 'Terminal: Focus terminal',
      key: 'Ctrl `',
      icon: Terminal,
      action: () => showTerminal(),
    },
    {
      label: 'Terminal: Run custom command',
      key: '',
      icon: Terminal,
      action: () => showModal('command'),
    },
    {
      label: 'Preferences: Settings',
      key: 'Ctrl ,',
      icon: Settings2,
      action: () => showModal('settings'),
    },
  ];
  const filteredCommands = $derived(
    commands.filter((command) =>
      command.label.toLowerCase().includes(input.toLowerCase()),
    ),
  );
  const contextItems = $derived.by((): MenuItem[] => {
    const entry = explorerMenu?.entry;
    if (!entry) return [];
    return [
      ...(entry.directory
        ? [
            {
              label: 'New file',
              key: '',
              icon: FilePlus2,
              action: () => newFile(entry.path),
              disabled: working,
            },
            {
              label: 'New folder',
              key: '',
              icon: FolderPlus,
              action: () => newFolder(entry.path),
              disabled: working,
            },
          ]
        : [
            {
              label: 'Open file',
              key: '',
              icon: FileCode2,
              action: () => openFile(entry),
            },
          ]),
      {
        label: 'Rename',
        key: 'F2',
        icon: Pencil,
        action: () => renameEntry(entry),
        disabled: working || entry.path === project?.path,
      },
      {
        label: 'Copy path',
        key: '',
        icon: Copy,
        action: async () => {
          await navigator.clipboard.writeText(entry.path);
          notify('Path copied');
        },
      },
      {
        label: 'Move to trash',
        key: '',
        icon: Trash2,
        action: () => {
          fileTarget = entry;
          trashTarget();
        },
        disabled: working || entry.path === project?.path,
      },
    ];
  });
  const menus = $derived.by((): MenuDefinition[] => {
    const editable = !!active && !active.diff && !active.external;
    const source = editable && !active?.scratch;
    const localProject = desktop && !!project;
    const file = commandGroup('File', {
      'Open folder': { disabled: !desktop || working },
      'Open file': { disabled: !desktop || working },
      'New file': { disabled: working },
      'New folder': { disabled: !localProject || working },
      Save: { disabled: !desktop || !editable || working },
      'Save all': { disabled: !desktop || !dirty || working },
      'Save as': {
        disabled: !localProject || !active || !!active.diff || working,
      },
    });
    const edit = commandGroup('Edit', {
      Undo: { disabled: !editable },
      Redo: { disabled: !editable },
      'Find and replace in file': { disabled: !active || !!active.diff },
      'Format document': { disabled: !desktop || !source },
    });
    const view = commandGroup('View', {
      'Split editor': {
        disabled: !active || !!active.diff,
        checked: !!splitId,
      },
      'Toggle sidebar': { checked: sidebarVisible },
      'Toggle bottom panel': { checked: panelVisible },
      'Toggle word wrap': { checked: wrap },
      'Toggle agent chat': { checked: agentVisible },
      'Markdown preview': {
        disabled:
          !active || language(active.name) !== 'Markdown' || !!active.diff,
        checked: markdown,
      },
    });
    return [
      {
        label: 'File',
        groups: [
          file.slice(0, 2),
          file.slice(2, 4),
          file.slice(4),
          commandGroup('Preferences'),
        ],
      },
      {
        label: 'Edit',
        groups: [
          edit.slice(0, 2),
          [
            ...edit.slice(2),
            ...commandGroup('Search', {
              'Find in project': { disabled: !localProject },
            }),
          ],
        ],
      },
      {
        label: 'View',
        groups: [
          view.slice(0, 3),
          view.slice(3),
          commandGroup('Source control', {
            'Open Git': { disabled: !localProject },
          }),
        ],
      },
      {
        label: 'Go',
        groups: [
          commandGroup('Go', {
            'Open file quickly': { disabled: !project },
            'Go to line': { disabled: !active || !!active.diff },
          }),
        ],
      },
      {
        label: 'Terminal',
        groups: [
          commandGroup('Terminal', {
            'New terminal': { disabled: !localProject },
            'Focus terminal': { disabled: !localProject },
            'Run custom command': { disabled: !localProject },
          }),
          commandGroup('Tasks', {
            'Run build task': { disabled: !localProject || building },
            'Run task': { disabled: !localProject },
            'Run active file': { disabled: !localProject || !source },
            'Configure tasks.json': { disabled: !localProject },
          }),
        ],
      },
    ];
  });
  const quickFiles = $derived(
    index
      .filter((entry) =>
        relative(entry.path).toLowerCase().includes(input.toLowerCase()),
      )
      .slice(0, 100),
  );
  const filteredTasks = $derived(
    tasks.filter((task) =>
      task.label.toLowerCase().includes(input.toLowerCase()),
    ),
  );

  function relative(path: string) {
    return project ? path.slice(project.path.length + 1) : path;
  }
  function commandGroup(
    category: string,
    options: Record<
      string,
      Partial<Pick<MenuItem, 'disabled' | 'checked'>>
    > = {},
  ): MenuItem[] {
    return commands
      .filter((command) => command.label.startsWith(category + ': '))
      .map((command) => {
        const label = command.label.slice(category.length + 2);
        return { ...command, label, ...options[label] };
      });
  }
  function message(error: unknown) {
    return error instanceof Error ? error.message : String(error);
  }
  function notify(text: string) {
    notice = text;
    clearTimeout(noticeTimer);
    noticeTimer = setTimeout(() => (notice = ''), 6500);
  }
  function report(error: unknown) {
    notify(message(error));
  }
  function needsSave(tab: Tab) {
    return (
      !tab.diff &&
      !tab.external &&
      (tab.content !== tab.saved || (tab.scratch && tab.content.length > 0))
    );
  }
  function showActivity(value: typeof activity) {
    activity = value;
    sidebarVisible = true;
  }
  function showModal(value: typeof modal, valueInput = '') {
    input = valueInput;
    selection = 0;
    modalError = '';
    modal = value;
  }
  function confirm(text: string, action: () => void | Promise<void>) {
    confirmText = text;
    pendingAction = action;
    showModal('confirm');
  }
  function guard(action: () => void | Promise<void>, check = dirty) {
    if (check) confirm('Discard unsaved changes and continue?', action);
    else void Promise.resolve(action()).catch(report);
  }
  async function confirmAction() {
    const action = pendingAction;
    pendingAction = null;
    modal = null;
    try {
      await action?.();
    } catch (error) {
      report(error);
    }
  }
  function closeModal() {
    if (working) return;
    modal = null;
    pendingAction = null;
    saveTarget = null;
    fileTarget = null;
  }
  function mountDialog(node: HTMLDialogElement) {
    node.showModal();
    node.querySelector('input')?.focus();
    return {
      destroy() {
        node.close();
      },
    };
  }
  function addDocument(document: Document) {
    const existing = tabs.find(
      (tab) => tab.path === document.path && !tab.diff,
    );
    if (existing) {
      activeId = existing.id;
      return existing.id;
    }
    const id = crypto.randomUUID();
    tabs = [
      ...tabs,
      { ...document, id, saved: document.content, scratch: false },
    ];
    activeId = id;
    return id;
  }
  async function openFile(
    entry: Pick<Entry, 'path'>,
    line?: number,
    column?: number,
    utf16 = false,
  ) {
    try {
      const existing = tabs.find((tab) => tab.path === entry.path && !tab.diff);
      const outside =
        utf16 && project && !entry.path.startsWith(project.path + '/');
      const id =
        existing?.id ??
        addDocument(
          outside
            ? await backend().ReadSource(entry.path)
            : await backend().ReadFile(entry.path),
        );
      if (outside)
        tabs = tabs.map((tab) =>
          tab.id === id ? { ...tab, external: true } : tab,
        );
      activeId = id;
      markdown = false;
      await tick();
      editors[id]?.focus();
      if (line) {
        if (utf16) {
          const text =
            tabs.find((tab) => tab.id === id)?.content.split('\n')[line - 1] ??
            '';
          column = [...text.slice(0, (column ?? 1) - 1)].length + 1;
        }
        editors[id]?.goTo(line, column);
      }
    } catch (error) {
      report(error);
    }
  }
  async function loadWorkspace(next: Project) {
    if (project) localStorage.removeItem('praewa-drafts:' + project.path);
    project = next;
    selectedEntry = null;
    explorerEdit = null;
    explorerMenu = null;
    restoreDrafts = [];
    tabs = [];
    activeId = '';
    splitId = '';
    markdown = false;
    diagnostics = {};
    languageStatuses = {};
    revision++;
    activity = 'explorer';
    recent = [next.path, ...recent.filter((path) => path !== next.path)].slice(
      0,
      10,
    );
    await refresh();
  }
  function openFolder(path?: string) {
    guard(async () => {
      working = true;
      try {
        const next = path
          ? await backend().OpenWorkspace(path)
          : await backend().OpenFolder();
        if (next) await loadWorkspace(next);
      } catch (error) {
        report(error);
      } finally {
        working = false;
      }
    });
  }
  async function openDialog() {
    try {
      const file = await backend().ChooseFile();
      if (!file) return;
      const outside = !project || !file.path.startsWith(project.path + '/');
      const action = async () => {
        if (outside)
          await loadWorkspace(await backend().OpenWorkspace(file.parent));
        await openFile({ path: file.path });
      };
      guard(action, outside && dirty);
    } catch (error) {
      report(error);
    }
  }
  function playground() {
    guard(async () => {
      if (!desktop) {
        const id = crypto.randomUUID();
        tabs = [
          ...tabs,
          {
            id,
            name: 'main.go',
            path: '',
            content: starter,
            version: '',
            saved: starter,
            scratch: true,
          },
        ];
        activeId = id;
        return;
      }
      working = true;
      try {
        const next = await backend().CreatePlayground();
        await loadWorkspace(next);
        const file = next.entries.find((entry) => entry.name === 'main.go');
        if (file) await openFile(file);
      } catch (error) {
        report(error);
      } finally {
        working = false;
      }
    });
  }
  async function refreshGit() {
    if (desktop && project)
      try {
        git = await backend().GitStatus();
      } catch (error) {
        report(error);
      }
  }
  async function refreshTasks() {
    if (desktop && project)
      try {
        tasks = await backend().Tasks(
          active?.scratch || active?.external ? '' : (active?.path ?? ''),
        );
      } catch (error) {
        report(error);
      }
  }
  async function refresh() {
    if (!project || !desktop) return;
    const root = project.path;
    try {
      const [entries, files, state, nextTasks] = await Promise.all([
        backend().ListDirectory(root),
        backend().FileIndex(),
        backend().GitStatus(),
        backend().Tasks(
          active?.scratch || active?.external ? '' : (active?.path ?? ''),
        ),
      ]);
      if (project?.path !== root) return;
      project = { ...project, entries };
      index = files;
      git = state;
      tasks = nextTasks;
      revision++;
    } catch (error) {
      report(error);
    }
  }
  async function diskChanged() {
    const root = project?.path;
    if (!root) return;
    await refresh();
    for (const tab of tabs.filter(
      (tab) => !tab.scratch && !tab.diff && !tab.external,
    )) {
      try {
        const document = await backend().ReadFile(tab.path);
        if (project?.path !== root) return;
        tabs = tabs.map((current) =>
          current.id !== tab.id || current.path !== tab.path
            ? current
            : document.version === current.version
              ? { ...current, missing: false }
              : needsSave(current)
                ? { ...current, conflict: true, missing: false }
                : {
                    ...current,
                    ...document,
                    saved: document.content,
                    conflict: false,
                    missing: false,
                  },
        );
      } catch {
        if (project?.path === root)
          tabs = tabs.map((current) =>
            current.id === tab.id && current.path === tab.path
              ? { ...current, missing: true }
              : current,
          );
      }
    }
  }
  function closeTab(tab: Tab) {
    guard(() => {
      const at = tabs.findIndex((item) => item.id === tab.id);
      tabs = tabs.filter((item) => item.id !== tab.id);
      if (activeId === tab.id)
        activeId = tabs[Math.min(at, tabs.length - 1)]?.id ?? '';
      if (splitId === tab.id) splitId = '';
      delete editors[tab.id];
      try {
        localStorage.setItem(
          'praewa-drafts:' + project?.path,
          JSON.stringify(tabs.filter(needsSave)),
        );
      } catch {}
      if (desktop && tab.path && !tab.diff)
        void backend()
          .LanguageClose(tab.path)
          .catch(() => {});
    }, needsSave(tab));
  }
  function selectedParent() {
    if (
      !selectedEntry ||
      !project ||
      (selectedEntry.path !== project.path &&
        !selectedEntry.path.startsWith(project.path + '/'))
    )
      return project?.path ?? '';
    return selectedEntry.directory
      ? selectedEntry.path
      : selectedEntry.path.slice(0, selectedEntry.path.lastIndexOf('/'));
  }
  function startEntry(mode: 'file' | 'folder', parent: string) {
    if (working) return;
    showActivity('explorer');
    explorerMenu = null;
    modal = null;
    explorerEdit = { id: crypto.randomUUID(), mode, parent };
  }
  function newFile(parent = selectedParent()) {
    if (project && desktop) startEntry('file', parent);
    else {
      const id = crypto.randomUUID();
      const name = `untitled-${tabs.filter((tab) => tab.scratch).length + 1}.go`;
      tabs = [
        ...tabs,
        {
          id,
          name,
          path: '',
          content: '',
          saved: '',
          version: '',
          scratch: true,
        },
      ];
      activeId = id;
    }
  }
  function newFolder(parent = selectedParent()) {
    if (project && desktop) startEntry('folder', parent);
  }
  function renameEntry(entry: Entry) {
    if (working || entry.path === project?.path) return;
    showActivity('explorer');
    explorerMenu = null;
    explorerEdit = {
      id: crypto.randomUUID(),
      mode: 'rename',
      parent: entry.path.slice(0, entry.path.lastIndexOf('/')),
      entry,
    };
  }
  async function moveEntry(entry: Entry, destination: string) {
    if (working)
      throw new Error('Wait for the current file operation to finish.');
    if (entry.path === destination) return;
    working = true;
    try {
      const next = await backend().Rename(entry.path, destination);
      for (const tab of tabs.filter(
        (tab) =>
          tab.path === entry.path || tab.path.startsWith(entry.path + '/'),
      )) {
        if (!tab.external)
          void backend()
            .LanguageClose(tab.path)
            .catch(() => {});
      }
      tabs = tabs.map((tab) =>
        !tab.external &&
        (tab.path === entry.path || tab.path.startsWith(entry.path + '/'))
          ? {
              ...tab,
              path: next.path + tab.path.slice(entry.path.length),
              name: tab.path === entry.path ? next.name : tab.name,
            }
          : tab,
      );
      selectedEntry = next;
      await refresh();
    } finally {
      working = false;
    }
  }
  async function commitEntry(request: ExplorerEdit, name: string) {
    const path = `${request.parent}/${name}`;
    if (request.mode === 'rename' && request.entry) {
      await moveEntry(request.entry, path);
      explorerEdit = null;
      return;
    }
    if (working)
      throw new Error('Wait for the current file operation to finish.');
    working = true;
    try {
      if (request.mode === 'folder')
        selectedEntry = await backend().CreateDirectory(path);
      else {
        const document = await backend().CreateFile(path, '');
        selectedEntry = {
          name: document.name,
          path: document.path,
          directory: false,
        };
        addDocument(document);
      }
      explorerEdit = null;
      await refresh();
    } finally {
      working = false;
    }
  }
  async function submitFile() {
    if (!input.trim()) {
      modalError = 'Enter a name.';
      return;
    }
    working = true;
    try {
      const path = newParent ? `${newParent}/${input.trim()}` : input.trim();
      if (modal === 'saveas' && saveTarget) {
        const target = tabs.find((tab) => tab.id === saveTarget?.id);
        if (!target) return;
        const document = await backend().CreateFile(path, target.content);
        if (target.scratch)
          tabs = tabs.map((tab) =>
            tab.id === target.id
              ? {
                  ...tab,
                  ...document,
                  saved: document.content,
                  scratch: false,
                  conflict: false,
                  missing: false,
                }
              : tab,
          );
        else addDocument(document);
      }
      modal = null;
      await refresh();
    } catch (error) {
      modalError = message(error);
    } finally {
      working = false;
    }
  }
  function saveAs(target = active) {
    if (!target || target.diff) return;
    if (!project) {
      notify('Open a workspace before saving this file.');
      return;
    }
    saveTarget = target;
    newParent = project.path;
    showModal('saveas', target.name);
  }
  async function saveTab(tab: Tab, force = false) {
    if (tab.diff || tab.external) return true;
    if (tab.scratch || tab.missing) {
      saveAs(tab);
      return false;
    }
    const content = tab.content;
    try {
      const document = await backend().SaveChecked(
        tab.path,
        content,
        force ? '' : tab.version,
      );
      tabs = tabs.map((current) =>
        current.id === tab.id
          ? {
              ...current,
              saved: content,
              version: document.version,
              conflict: false,
              missing: false,
            }
          : current,
      );
      return true;
    } catch (error) {
      report(error);
      return false;
    }
  }
  async function saveActive() {
    if (active?.external) {
      notify(
        'This source view is read only. Use Save as to create a copy in your workspace.',
      );
      return;
    }
    if (active && (await saveTab(active))) notify(`${active.name} saved`);
  }
  async function saveAll() {
    for (const tab of tabs.filter(needsSave)) {
      if (!(await saveTab(tab))) return false;
    }
    return true;
  }
  async function reload(tab: Tab) {
    guard(async () => {
      try {
        const document = await backend().ReadFile(tab.path);
        tabs = tabs.map((current) =>
          current.id === tab.id
            ? {
                ...current,
                ...document,
                saved: document.content,
                conflict: false,
                missing: false,
              }
            : current,
        );
      } catch (error) {
        report(error);
      }
    }, needsSave(tab));
  }
  function updateContent(id: string, content: string) {
    tabs = tabs.map((tab) => (tab.id === id ? { ...tab, content } : tab));
  }
  function context(entry: Entry, event: MouseEvent) {
    fileTarget = entry;
    selectedEntry = entry;
    const rect = (event.currentTarget as HTMLElement).getBoundingClientRect();
    explorerMenu = {
      entry,
      x: event.clientX || rect.left + 24,
      y: event.clientY || rect.bottom,
    };
  }
  function trashTarget() {
    if (!fileTarget) return;
    const target = fileTarget;
    confirm(
      `Move ${target.name} to workspace trash? You can undo this operation. Unsaved edits in this file or folder will be discarded.`,
      async () => {
        undoTrash = await backend().Trash(target.path);
        for (const tab of tabs.filter(
          (tab) =>
            tab.path === target.path || tab.path.startsWith(target.path + '/'),
        ))
          void backend()
            .LanguageClose(tab.path)
            .catch(() => {});
        tabs = tabs.filter(
          (tab) =>
            tab.path !== target.path && !tab.path.startsWith(target.path + '/'),
        );
        if (!tabs.some((tab) => tab.id === activeId))
          activeId = tabs[0]?.id ?? '';
        if (!tabs.some((tab) => tab.id === splitId)) splitId = '';
        await refresh();
        notify(`${target.name} moved to trash`);
      },
    );
  }
  async function restoreTrash() {
    if (!undoTrash) return;
    try {
      await backend().RestoreTrash(undoTrash);
      undoTrash = null;
      await refresh();
      notify('File restored');
    } catch (error) {
      report(error);
    }
  }
  function toggleSplit() {
    if (splitId) splitId = '';
    else if (active && !active.diff) {
      splitId = active.id;
      markdown = false;
    }
  }
  async function format() {
    if (!active || active.scratch || active.diff || active.external) return;
    try {
      const target = active;
      const edits = await backend().LanguageRequest(
        target.path,
        target.content,
        'formatting',
        0,
        0,
      );
      const text = Array.isArray(edits)
        ? applyEdits(target.content, edits)
        : await backend().Format(target.path, target.content);
      if (tabs.find((tab) => tab.id === target.id)?.content === target.content)
        updateContent(target.id, text);
    } catch (error) {
      report(error);
    }
  }
  function addTerminal(info: TerminalInfo) {
    terminals = [...terminals, info];
    terminalId = info.id;
    panel = 'terminal';
    panelVisible = true;
  }
  async function newTerminal() {
    try {
      addTerminal(
        await backend().NewTerminal({
          shell,
          cwd: project?.path ?? '',
          columns: 100,
          rows: 24,
        }),
      );
    } catch (error) {
      report(error);
    }
  }
  function showTerminal() {
    panel = 'terminal';
    panelVisible = true;
    if (!terminals.length && desktop) void newTerminal();
  }
  async function closeTerminal(info: TerminalInfo) {
    try {
      await backend().TerminalClose(info.id);
      const at = terminals.findIndex((item) => item.id === info.id);
      terminals = terminals.filter((item) => item.id !== info.id);
      if (terminalId === info.id)
        terminalId = terminals[Math.min(at, terminals.length - 1)]?.id ?? '';
    } catch (error) {
      report(error);
    }
  }
  async function runTask(task: Task) {
    if (task.problem) {
      notify(task.problem);
      return;
    }
    if (!(await saveAll())) return;
    try {
      addTerminal(
        await backend().RunTask(
          task.id,
          active?.scratch || active?.external ? '' : (active?.path ?? ''),
        ),
      );
      modal = null;
    } catch (error) {
      report(error);
    }
  }
  async function compile() {
    if (building) return;
    if (!(await saveAll())) return;
    await refreshTasks();
    const defaults = tasks.filter((task) => task.default && !task.problem);
    if (defaults.length === 1) {
      await runTask(defaults[0]);
      return;
    }
    if (tasks.some((task) => !task.problem && task.id !== 'file:run')) {
      showModal('task');
      return;
    }
    if (!active || active.scratch || active.diff || active.external) {
      notify('Open a source file or configure a build task.');
      return;
    }
    panel = 'output';
    panelVisible = true;
    output = '';
    building = true;
    try {
      const result = await backend().Build(active.path);
      output = `$ ${result.command}\n\n${result.output}\n${result.artifact ? `Artifact: ${result.artifact}\n` : ''}Build ${result.cancelled ? 'stopped' : result.exitCode === 0 ? 'completed' : 'failed'} · exit ${result.exitCode}\n`;
    } catch (error) {
      output += message(error) + '\n';
    } finally {
      building = false;
    }
  }
  async function runActive() {
    await refreshTasks();
    const task = tasks.find((task) => task.id === 'file:run');
    if (task) await runTask(task);
    else showModal('task');
  }
  async function runCommand() {
    if (!input.trim()) return;
    if (!(await saveAll())) return;
    try {
      addTerminal(await backend().RunCommand(input));
      modal = null;
    } catch (error) {
      modalError = message(error);
    }
  }
  async function configureTasks() {
    if (!project) return;
    const folder = project.path + '/.vscode';
    const path = folder + '/tasks.json';
    try {
      try {
        await backend().ListDirectory(folder);
      } catch {
        await backend().CreateDirectory(folder);
      }
      try {
        addDocument(await backend().ReadFile(path));
      } catch {
        addDocument(
          await backend().CreateFile(
            path,
            JSON.stringify(
              {
                version: '2.0.0',
                tasks: [
                  {
                    label: 'Build',
                    type: 'shell',
                    command: 'go build ./...',
                    group: { kind: 'build', isDefault: true },
                    options: { cwd: '${workspaceFolder}' },
                  },
                ],
              },
              null,
              2,
            ) + '\n',
          ),
        );
      }
      await refresh();
    } catch (error) {
      report(error);
    }
  }
  function replaceProject(
    query: SearchQuery,
    replacement: string,
    matches: SearchMatch[],
  ) {
    const files = [
      ...new Map(matches.map((match) => [match.path, match.version])).entries(),
    ];
    if (
      tabs.some(
        (tab) => needsSave(tab) && files.some(([path]) => path === tab.path),
      )
    ) {
      notify(
        'Save the affected open files, then search again before replacing.',
      );
      return;
    }
    confirm(
      `Replace ${matches.length} matches in ${files.length} files on disk? Review the search results before continuing.`,
      async () => {
        let count = 0;
        for (const [path, version] of files) {
          const document = await backend().ReplaceSearchFile(
            query,
            path,
            replacement,
            version,
          );
          tabs = tabs.map((tab) =>
            tab.path === path && !tab.diff
              ? { ...tab, ...document, saved: document.content }
              : tab,
          );
          count++;
        }
        await diskChanged();
        notify(`Replaced matches in ${count} files`);
      },
    );
  }
  async function stage(path: string, value: boolean) {
    working = true;
    try {
      await backend().GitStage(path, value);
      await refreshGit();
    } catch (error) {
      report(error);
    } finally {
      working = false;
    }
  }
  async function diff(path: string, value: boolean) {
    try {
      const document = await backend().GitDiff(path, value);
      const id = crypto.randomUUID();
      tabs = [
        ...tabs,
        {
          id,
          name: document.name + (value ? ' (staged diff)' : ' (diff)'),
          path,
          content: '',
          saved: '',
          version: '',
          scratch: false,
          diff: document,
        },
      ];
      activeId = id;
      markdown = false;
    } catch (error) {
      report(error);
    }
  }
  async function commit(message: string) {
    working = true;
    try {
      const text = await backend().GitCommit(message);
      notify(text);
      await refreshGit();
      return true;
    } catch (error) {
      report(error);
      return false;
    } finally {
      working = false;
    }
  }
  function initGit() {
    confirm('Initialize a Git repository in this workspace?', async () => {
      await backend().GitInit();
      await refreshGit();
    });
  }
  function resize(event: PointerEvent, kind: 'sidebar' | 'panel' | 'agent') {
    event.preventDefault();
    const start =
      kind === 'sidebar'
        ? sidebarWidth
        : kind === 'agent'
          ? agentWidth
          : panelHeight;
    const pointer = kind === 'panel' ? event.clientY : event.clientX;
    const move = (event: PointerEvent) => {
      if (kind === 'sidebar')
        sidebarWidth = Math.max(
          180,
          Math.min(450, start + event.clientX - pointer),
        );
      else if (kind === 'agent')
        agentWidth = Math.max(
          280,
          Math.min(520, start + pointer - event.clientX),
        );
      else
        panelHeight = Math.max(
          120,
          Math.min(window.innerHeight - 240, start + pointer - event.clientY),
        );
    };
    const up = () => {
      window.removeEventListener('pointermove', move);
      window.removeEventListener('pointerup', up);
    };
    window.addEventListener('pointermove', move);
    window.addEventListener('pointerup', up);
  }
  function executeSelected() {
    if (modal === 'commands') {
      const command = filteredCommands[selection];
      modal = null;
      void Promise.resolve(command?.action()).catch(report);
    } else if (modal === 'quick') {
      const file = quickFiles[selection];
      modal = null;
      if (file) void openFile(file);
    } else if (modal === 'task') {
      const task = filteredTasks[selection];
      if (task) void runTask(task);
    }
  }
  function paletteKey(event: KeyboardEvent) {
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault();
      const count =
        modal === 'quick'
          ? quickFiles.length
          : modal === 'task'
            ? filteredTasks.length
            : filteredCommands.length;
      selection = Math.max(
        0,
        Math.min(count - 1, selection + (event.key === 'ArrowDown' ? 1 : -1)),
      );
    } else if (event.key === 'Enter') {
      event.preventDefault();
      executeSelected();
    }
  }
  function shortcuts(event: KeyboardEvent) {
    if (event.defaultPrevented) return;
    if (
      (event.ctrlKey || event.metaKey) &&
      event.altKey &&
      event.key.toLowerCase() === 'a' &&
      !modal
    ) {
      event.preventDefault();
      agentVisible = !agentVisible;
      return;
    }
    if (event.key === 'Escape' && modal) {
      closeModal();
      return;
    }
    if (modal) return;
    if (event.altKey && event.key.toLowerCase() === 'z') {
      event.preventDefault();
      wrap = !wrap;
      return;
    }
    if (event.altKey && event.shiftKey && event.key.toLowerCase() === 'f') {
      event.preventDefault();
      void format();
      return;
    }
    if (!(event.ctrlKey || event.metaKey)) return;
    const key = event.key.toLowerCase();
    if (key === 's') {
      event.preventDefault();
      if (event.shiftKey) void saveAll();
      else void saveActive();
    } else if (key === 'o') {
      event.preventDefault();
      openFolder();
    } else if (key === 'n') {
      event.preventDefault();
      newFile();
    } else if (key === 'p') {
      event.preventDefault();
      showModal(event.shiftKey ? 'commands' : 'quick');
    } else if (key === 'b') {
      event.preventDefault();
      if (event.shiftKey) void compile();
      else sidebarVisible = !sidebarVisible;
    } else if (key === 'j') {
      event.preventDefault();
      panelVisible = !panelVisible;
    } else if (key === 'f' && event.shiftKey) {
      event.preventDefault();
      showActivity('search');
    } else if (key === 'g') {
      event.preventDefault();
      showModal('line', String(cursor.line));
    } else if (key === '\\') {
      event.preventDefault();
      toggleSplit();
    } else if (key === '`' || key === '~') {
      event.preventDefault();
      if (event.shiftKey) void newTerminal();
      else showTerminal();
    } else if (key === 'enter') {
      event.preventDefault();
      void runActive();
    } else if (key === ',') {
      event.preventDefault();
      showModal('settings');
    } else if (key === 'w' && active) {
      event.preventDefault();
      closeTab(active);
    } else if (key === 'tab' && tabs.length) {
      event.preventDefault();
      const i = tabs.findIndex((tab) => tab.id === activeId);
      activeId =
        tabs[(i + (event.shiftKey ? tabs.length - 1 : 1)) % tabs.length].id;
    }
  }
  onMount(() => {
    desktop = isDesktop();
    try {
      const settings = JSON.parse(
        localStorage.getItem('praewa-workbench') ?? '{}',
      );
      fontSize = Math.max(11, Math.min(24, settings.fontSize ?? 14));
      terminalFontSize = Math.max(
        10,
        Math.min(22, settings.terminalFontSize ?? 13),
      );
      wrap = !!settings.wrap;
      tabSize = [2, 4, 8].includes(settings.tabSize) ? settings.tabSize : 4;
      insertSpaces = !!settings.insertSpaces;
      sidebarWidth = Math.max(180, Math.min(450, settings.sidebarWidth ?? 250));
      agentWidth = Math.max(280, Math.min(520, settings.agentWidth ?? 340));
      agentVisible =
        typeof settings.agentVisible === 'boolean'
          ? settings.agentVisible
          : window.innerWidth >= 1000;
      panelHeight = Math.max(
        120,
        Math.min(window.innerHeight - 240, settings.panelHeight ?? 230),
      );
    } catch {}
    const unsubscribeDiagnostics = onEvent<DiagnosticsEvent>(
      'language:diagnostics',
      (event) => {
        if (project && event.path.startsWith(project.path + '/'))
          diagnostics = { ...diagnostics, [event.path]: event };
      },
    );
    const unsubscribeOutput = onBuildOutput((text) => (output += text));
    let debounce: ReturnType<typeof setTimeout>;
    const unsubscribeWatch = onEvent<string>('workspace:changed', () => {
      clearTimeout(debounce);
      debounce = setTimeout(() => {
        void diskChanged();
      }, 200);
    });
    void (async () => {
      if (desktop)
        try {
          const state = await backend().InitialState();
          shells = state.shells;
          shell = state.shells.find((profile) => profile.default)?.path ?? '';
          recent = state.session.recent;
          if (state.project) {
            await loadWorkspace(state.project);
            for (const path of state.session.openFiles)
              try {
                addDocument(await backend().ReadFile(path));
              } catch {}
            const selected = tabs.find(
              (tab) => tab.path === state.session.activeFile,
            );
            if (selected) activeId = selected.id;
          }
          const drafts = JSON.parse(
            localStorage.getItem('praewa-drafts:' + project?.path) ?? '[]',
          ) as Tab[];
          restoreDrafts = drafts.filter(
            (tab) =>
              tab.scratch ||
              (tab.path.startsWith((project?.path ?? '') + '/') &&
                tab.content !== tab.saved),
          );
        } catch (error) {
          report(error);
        }
      ready = true;
    })();
    return () => {
      unsubscribeDiagnostics();
      unsubscribeOutput();
      unsubscribeWatch();
      clearTimeout(debounce);
      clearTimeout(noticeTimer);
    };
  });
  $effect(() => {
    if (desktop)
      void backend()
        .SetDirty(dirty)
        .catch(() => {});
  });
  $effect(() => {
    localStorage.setItem(
      'praewa-workbench',
      JSON.stringify({
        fontSize,
        terminalFontSize,
        wrap,
        tabSize,
        insertSpaces,
        sidebarWidth,
        panelHeight,
        agentVisible,
        agentWidth,
      }),
    );
  });
  $effect(() => {
    const files = tabs
      .filter((tab) => !tab.scratch && !tab.diff && !tab.external)
      .map((tab) => tab.path);
    const selected = active?.diff ? '' : (active?.path ?? '');
    const drafts = tabs.filter(needsSave).filter((tab) => !tab.diff);
    const root = project?.path;
    const recovery = restoreDrafts;
    if (ready) {
      const timer = setTimeout(() => {
        if (desktop && root)
          void backend().SaveSession(files, selected).catch(report);
        try {
          localStorage.setItem(
            'praewa-drafts:' + root,
            JSON.stringify([
              ...recovery.filter(
                (draft) =>
                  !drafts.some(
                    (tab) =>
                      tab.id === draft.id ||
                      (tab.path && tab.path === draft.path),
                  ),
              ),
              ...drafts,
            ]),
          );
        } catch {
          notify(
            'Draft storage is full. Save your files to keep your changes.',
          );
        }
      }, 500);
      return () => clearTimeout(timer);
    }
  });
  $effect(() => {
    const id = activeId;
    markdown = false;
    if (ready)
      untrack(() => {
        void refreshTasks();
        void tick().then(() => editors[id]?.focus());
      });
  });
  $effect(() => {
    output;
    if (consoleElement)
      void tick().then(() => {
        if (consoleElement)
          consoleElement.scrollTop = consoleElement.scrollHeight;
      });
  });
</script>

<svelte:window
  onkeydown={shortcuts}
  onbeforeunload={(event) => {
    if (dirty && !desktop) event.preventDefault();
  }}
/>
<div class="app-shell h-dvh flex flex-col min-w-0">
  <header
    class="topbar h-[54px] shrink-0 flex items-center gap-[15px] pt-0 pr-[20px] pb-0 pl-[15px] bg-[#0d0e10] border-b border-b-line"
  >
    <button
      class="brand flex items-center gap-[10px] text-[13px] font-bold tracking-[1.2px] p-0 whitespace-nowrap"
      onclick={() => (activeId = '')}
      title="Welcome"
      ><span
        class="brand-mark flex items-center justify-center w-[29px] h-[29px] bg-accent text-[#121314] rounded-[7px]"
        ><Code2 size={21} /></span
      ><span
        >PRAEWA<span class="brand-code text-[#858a94] font-[450] ml-[5px]"
          >CODE</span
        ></span
      ></button
    >
    <MenuBar {menus} onerror={report} />
    <button
      class="command-trigger flex items-center gap-[9px] bg-[#17181c] border border-[#292b31] rounded-[6px] h-[29px] w-[300px] text-[#7e838c] text-[11px] ml-auto py-0 px-[10px] min-w-[80px]"
      onclick={() => showModal('commands')}
      ><Search size={13} /><span>{project?.name ?? 'Search commands'}</span><kbd
        >Ctrl ⇧ P</kbd
      ></button
    >
    <button
      class="agent-toggle flex items-center gap-[6px] text-[11px] text-[#949ba8] py-[6px] px-[8px] border border-[#2f3137] rounded-[5px] shrink-0"
      class:open={agentVisible}
      aria-label="Toggle agent chat"
      aria-pressed={agentVisible}
      title="Agent chat · Ctrl Alt A"
      onclick={() => (agentVisible = !agentVisible)}
      ><MessageSquare size={15} /><span>Agent</span></button
    >
    <span
      class="local-badge flex items-center gap-[6px] border border-[#32352c] bg-[#191c16] text-[#a4b695] rounded-[4px] text-[9px] font-semibold tracking-[0.8px] py-[4px] px-[7px]"
      ><span
        class="status-dot inline-block w-[5px] h-[5px] rounded-[50%] bg-[#92b58a] shrink-0"
      ></span>{desktop ? 'LOCAL' : 'PREVIEW'}</span
    >
  </header>
  <div class="workbench relative flex flex-1 min-h-0">
    <nav
      class="activitybar w-[55px] bg-[#0e0f11] border-r border-r-line flex items-center flex-col pt-[10px] gap-[5px] shrink-0"
      aria-label="Workbench navigation"
    >
      {#each [{ id: 'explorer', label: 'Explorer', icon: Files }, { id: 'search', label: 'Search project', icon: Search }, { id: 'git', label: 'Source control', icon: GitBranch }, { id: 'tasks', label: 'Tasks', icon: Hammer }] as item}<button
          class="activity-button relative w-[54px] h-[44px] flex items-center justify-center text-[#686e79]"
          class:selected={activity === item.id && sidebarVisible}
          title={item.label}
          aria-label={item.label}
          onclick={() => {
            if (activity === item.id) sidebarVisible = !sidebarVisible;
            else showActivity(item.id as typeof activity);
          }}
          ><item.icon
            size={21}
          />{#if item.id === 'git' && git?.files.length}<span
              class="activity-count absolute right-[5px] bottom-[4px] bg-[#d8bd4a] text-[#131417] text-[8px] py-[2px] px-[4px] rounded-[8px]"
              >{git.files.length}</span
            >{/if}</button
        >{/each}
      <div class="activity-bottom">
        <button
          class="activity-button relative w-[54px] h-[44px] flex items-center justify-center text-[#686e79]"
          class:selected={agentVisible}
          title="Agent chat"
          aria-label="Agent chat"
          aria-pressed={agentVisible}
          onclick={() => (agentVisible = !agentVisible)}
          ><MessageSquare size={21} /></button
        >
        <button
          class="activity-button relative w-[54px] h-[44px] flex items-center justify-center text-[#686e79]"
          title="Terminal"
          aria-label="Focus terminal"
          onclick={showTerminal}><Terminal size={21} /></button
        ><button
          class="activity-button relative w-[54px] h-[44px] flex items-center justify-center text-[#686e79]"
          title="Settings"
          aria-label="Settings"
          onclick={() => showModal('settings')}><Settings2 size={21} /></button
        >
      </div>
    </nav>
    {#if sidebarVisible}
      <aside
        class="sidebar w-[235px] bg-[#141518] border-r border-r-line flex flex-col shrink-0 overflow-hidden min-w-[180px]"
        style:width={`${sidebarWidth}px`}
      >
        <div
          class="sidebar-heading h-[48px] flex justify-between items-center pt-0 pr-[14px] pb-0 pl-[17px] text-[10px] font-semibold text-[#858a95] tracking-[1.35px]"
        >
          <span
            >{activity === 'explorer'
              ? 'EXPLORER'
              : activity === 'search'
                ? 'SEARCH'
                : activity === 'git'
                  ? 'SOURCE CONTROL'
                  : 'TASKS'}</span
          >
          <div class="sidebar-actions">
            <button
              class="icon-button"
              title="Refresh workspace"
              onclick={refresh}><RefreshCw size={13} /></button
            >{#if activity === 'explorer'}<button
                class="icon-button"
                title="New file"
                disabled={working}
                onclick={() => newFile()}><FilePlus2 size={14} /></button
              ><button
                class="icon-button"
                title="New folder"
                disabled={!project || working}
                onclick={() => newFolder()}><FolderPlus size={14} /></button
              >{/if}
          </div>
        </div>
        {#if activity === 'explorer'}
          {#if project}<FileTree
              root={project.path}
              label={project.name}
              entries={project.entries}
              active={active?.path ?? ''}
              selected={selectedEntry?.path ?? ''}
              refresh={revision}
              edit={explorerEdit}
              busy={working}
              onopen={openFile}
              onselect={(entry) => (selectedEntry = entry)}
              onrename={renameEntry}
              oncommit={commitEntry}
              oncancel={() => (explorerEdit = null)}
              onmove={(entry, parent) =>
                moveEntry(entry, `${parent}/${entry.name}`)}
              onerror={report}
              oncontext={context}
            />{:else}<div
              class="no-workspace pt-[34px] pr-[15px] pb-[27px] pl-[15px] text-center"
            >
              <FolderOpen size={36} />
              <h3>Open a workspace</h3>
              <p>Your files, tools and terminal in one place.</p>
              <button
                class="sidebar-open text-[11px] w-full mt-[21px] py-[9px] px-[13px] border border-[#49412a] bg-[#f5d54705] rounded-[5px] flex items-center justify-center gap-[30px] text-[#d4bf6f]"
                onclick={() => openFolder()}
                >Open folder<ChevronRight size={14} /></button
              >
            </div>{/if}
          {#if tabs.length}<div
              class="open-editors border-t border-t-line pt-[13px] pr-0 pb-[7px] pl-0 max-h-[230px] overflow-auto"
            >
              <div
                class="section-label flex items-center justify-between pt-0 pr-[17px] pb-[9px] pl-[17px] text-[9px] tracking-[1.1px] text-[#6f7580] font-semibold"
              >
                OPEN EDITORS<span>{tabs.length}</span>
              </div>
              {#each tabs as tab}<button
                  class="open-editor-entry flex items-center gap-[9px] w-full py-[8px] px-[19px] text-[11px] text-[#858b96] text-left"
                  class:current={tab.id === activeId}
                  onclick={() => (activeId = tab.id)}
                  ><FileIcon name={tab.name} /><span>{tab.name}</span
                  >{#if needsSave(tab)}<span
                      class="dirty-dot inline-block h-[6px] w-[6px] rounded-[50%] bg-[#d6c37b] shrink-0"
                    ></span>{/if}</button
                >{/each}
            </div>{/if}
          {#if outline.length}<div
              class="outline max-h-[170px] overflow-auto border-t border-t-[#2b2d34]"
            >
              <div class="list-section">OUTLINE</div>
              {#each outline as symbol}<button
                  onclick={() =>
                    active && editors[active.id]?.goTo(symbol.line)}
                  ><Code2 size={12} /><span>{symbol.name}</span><small
                    >{symbol.line}</small
                  ></button
                >{/each}
            </div>{/if}
        {:else if activity === 'search'}<SearchPanel
            root={project?.path ?? ''}
            {revision}
            onopen={(match) =>
              openFile({ path: match.path }, match.line, match.column)}
            onreplace={replaceProject}
            onerror={report}
          />
        {:else if activity === 'git'}<GitPanel
            gitState={git}
            root={project?.path ?? ''}
            busy={working}
            onstage={stage}
            ondiff={diff}
            oncommit={commit}
            oninit={initGit}
            onrefresh={refreshGit}
          />
        {:else}<div class="panel-form">
            <button
              class="secondary-button full-width"
              disabled={!project}
              onclick={configureTasks}
              ><Settings2 size={13} />Configure tasks.json</button
            ><button
              class="secondary-button full-width"
              disabled={!project}
              onclick={() => showModal('command')}
              ><Terminal size={13} />Run command…</button
            >
            <p class="panel-caption">
              Uses build tools installed on your computer.
            </p>
          </div>
          <div class="panel-scroll">
            {#each tasks as task}<button
                class="task-entry flex items-center gap-[9px] w-full py-[10px] px-[13px] text-left text-[11px] text-[#abb4c2]"
                title={task.problem ||
                  `${task.command} ${(task.args ?? []).join(' ')}\n${task.cwd}`}
                disabled={!!task.problem}
                onclick={() => runTask(task)}
                ><Play size={13} /><span
                  >{task.label}<small
                    >{task.source}{task.default
                      ? ' · default build'
                      : ''}</small
                  ></span
                ></button
              >{/each}{#if !tasks.length}<p class="panel-caption panel-form">
                Open a project with npm scripts, a Makefile, Go, Cargo or
                tasks.json.
              </p>{/if}
          </div>{/if}
      </aside>
      <button
        class="sidebar-resizer w-[4px] ml-[-2px] mr-[-2px] cursor-col-resize z-[2]"
        type="button"
        aria-label="Resize sidebar"
        tabindex="0"
        onpointerdown={(event) => resize(event, 'sidebar')}
        onkeydown={(event) => {
          if (event.key === 'ArrowLeft')
            sidebarWidth = Math.max(180, sidebarWidth - 10);
          if (event.key === 'ArrowRight')
            sidebarWidth = Math.min(450, sidebarWidth + 10);
        }}
      ></button>
    {/if}
    <main class="main-area flex flex-col flex-1 min-w-0 min-h-0 bg-surface">
      <div
        class="editor-toolbar h-[45px] flex items-stretch border-b border-b-line bg-[#131417] shrink-0"
      >
        <div class="tabs">
          {#each tabs as tab}<div
              class="tab-group flex border-r border-r-line border-t-2 border-t-[transparent] shrink-0 text-[#7f8591]"
              class:active={tab.id === activeId}
            >
              <button
                class="file-tab flex items-center gap-[9px] text-[11px] pt-0 pr-[13px] pb-0 pl-[17px] whitespace-nowrap"
                onclick={() => {
                  activeId = tab.id;
                  markdown = false;
                }}
                title={tab.path || 'Unsaved file'}
                ><FileIcon name={tab.name} /><span>{tab.name}</span
                >{#if needsSave(tab)}<span
                    class="dirty-dot inline-block h-[6px] w-[6px] rounded-[50%] bg-[#d6c37b] shrink-0"
                  ></span>{/if}</button
              ><button
                class="tab-close w-[27px] pr-[10px] flex justify-center items-center text-[#6c717d]"
                title={`Close ${tab.name}`}
                aria-label={`Close ${tab.name}`}
                onclick={() => closeTab(tab)}><X size={12} /></button
              >
            </div>{/each}
        </div>
        <div
          class="editor-toolbar-actions flex items-center gap-[12px] py-0 px-[15px] bg-[#131417] shrink-0"
        >
          <button
            class="icon-button"
            title="Split editor"
            aria-label="Split editor"
            disabled={!active || !!active.diff}
            onclick={toggleSplit}><Columns2 size={16} /></button
          >{#if active?.name.endsWith('.md')}<button
              class="icon-button"
              title="Markdown preview"
              onclick={() => (markdown = !markdown)}><Eye size={16} /></button
            >{/if}<button
            class="icon-button"
            title="Run active file"
            aria-label="Run active file"
            disabled={!active || !desktop || !!active.diff || !!active.external}
            onclick={runActive}><Play size={16} /></button
          ><button
            class="compile-button flex gap-[7px] items-center py-[6px] px-[10px] text-[11px] font-semibold rounded-[4px] bg-accent text-[#26241a]"
            disabled={!desktop || !project || building}
            onclick={compile}><Hammer size={13} />Build</button
          >
        </div>
      </div>
      {#if restoreDrafts.length}<div class="recovery-banner">
          {restoreDrafts.length} unsaved drafts from the previous session.<button
            onclick={() => {
              for (const draft of restoreDrafts) {
                const existing = tabs.find(
                  (tab) => tab.path === draft.path && !tab.diff,
                );
                if (existing) {
                  tabs = tabs.map((tab) =>
                    tab.id === existing.id
                      ? {
                          ...tab,
                          content: draft.content,
                          conflict: tab.version !== draft.version,
                        }
                      : tab,
                  );
                } else tabs = [...tabs, { ...draft, id: crypto.randomUUID() }];
              }
              activeId = tabs[tabs.length - 1]?.id ?? '';
              restoreDrafts = [];
            }}>Restore</button
          ><button onclick={() => (restoreDrafts = [])}>Dismiss</button>
        </div>{/if}
      <div class="editor-region flex-1 min-h-0 relative overflow-hidden">
        {#if !active}<div class="welcome">
            <div class="welcome-content relative z-[1] max-w-[570px]">
              <div
                class="welcome-eyebrow flex gap-[9px] items-center text-[8px] font-medium tracking-[2.1px] text-[#85816d]"
              >
                <span></span>YOUR LOCAL DEVELOPMENT WORKSPACE
              </div>
              <div class="welcome-symbol"><Code2 size={48} /></div>
              <h1>Make room<br />for <span>your ideas.</span></h1>
              <p
                class="welcome-description text-[#818792] text-[12px] leading-[1.9] m-0"
              >
                Edit, build and run. Your files, your tools, your terminal.<br
                />A focused workspace for the things you want to create.
              </p>
              <div
                class="welcome-actions flex items-center gap-[10px] mt-[27px]"
              >
                <button class="primary-button" onclick={() => openFolder()}
                  ><FolderOpen size={15} />Open folder<ChevronRight
                    size={14}
                  /></button
                ><button class="secondary-button" onclick={() => newFile()}
                  ><FilePlus2 size={15} />New file</button
                >
              </div>
              <button
                class="starter-link inline-flex gap-[7px] items-center text-[#8d918c] text-[10px] mt-[16px] py-[3px] px-0"
                onclick={playground}
                ><Code2 size={14} />{desktop
                  ? 'Create a Go playground'
                  : 'Try the editor'}<ChevronRight size={12} /></button
              >
              <div class="welcome-rule"></div>
              {#if recent.length}<div class="list-section">
                  RECENT WORKSPACES
                </div>
                {#each recent as path}<button
                    class="recent-workspace flex items-center gap-[10px] w-full py-[8px] px-0 text-[#9a9d90] text-[11px] text-left"
                    onclick={() => openFolder(path)}
                    ><FolderOpen size={13} /><span>{path}</span></button
                  >{/each}{:else}<div
                  class="quick-shortcuts flex gap-[28px] flex-wrap"
                >
                  <div><kbd>Ctrl P</kbd><span>Quick open</span></div>
                  <div><kbd>Ctrl ⇧ P</kbd><span>Commands</span></div>
                  <div><kbd>Ctrl `</kbd><span>Terminal</span></div>
                </div>{/if}
            </div>
          </div>{/if}
        <div class="editor-columns flex h-full min-w-0" class:hidden={!active}>
          <div
            class="editor-column flex-1 min-w-0 min-h-0 flex flex-col overflow-hidden"
          >
            {#each tabs as tab (tab.id)}<div
                class="document-pane h-full flex flex-col"
                class:hidden={tab.id !== activeId}
              >
                <div
                  class="breadcrumbs flex items-center gap-[8px] h-[34px] border-b border-b-[#ffffff03] py-0 px-[23px] text-[10px] text-[#737a86] overflow-hidden whitespace-nowrap shrink-0"
                >
                  <FolderOpen size={12} /><span
                    >{project?.name ?? 'Untitled'}</span
                  ><ChevronRight size={12} /><FileIcon
                    name={tab.name}
                    size={14}
                  /><span
                    >{tab.scratch
                      ? tab.name
                      : tab.external
                        ? tab.path
                        : relative(tab.path)}</span
                  >{#if tab.external}<span
                      class="unsaved-tag text-[9px] ml-[6px] text-[#8d8050] border border-[#3f3926] py-[1px] px-[5px] rounded-[3px]"
                      >READ ONLY</span
                    >{/if}{#if needsSave(tab)}<span
                      class="unsaved-tag text-[9px] ml-[6px] text-[#8d8050] border border-[#3f3926] py-[1px] px-[5px] rounded-[3px]"
                      >UNSAVED</span
                    >{/if}{#if splitId}<button
                      class="split-select"
                      onclick={() => (splitId = tab.id)}
                      >Open in right pane</button
                    >{/if}
                </div>
                {#if tab.conflict || tab.missing}<div class="conflict-banner">
                    <CircleAlert size={13} /><span
                      >{tab.missing
                        ? 'File was removed or renamed on disk.'
                        : 'File changed on disk. Your edits are preserved.'}</span
                    >{#if tab.missing}<button onclick={() => saveAs(tab)}
                        >Save as</button
                      >{:else}<button onclick={() => reload(tab)}>Reload</button
                      ><button
                        onclick={() =>
                          confirm(
                            'Overwrite the file on disk with your current edits?',
                            () => saveTab(tab, true).then(() => {}),
                          )}>Save anyway</button
                      >{/if}
                  </div>{/if}
                <div class="code-pane flex-1 min-h-0">
                  {#if tab.diff}<DiffView
                      diff={tab.diff}
                    />{:else if tab.id === activeId && markdown}<MarkdownPreview
                      content={tab.content}
                    />{:else}<Editor
                      bind:this={editors[tab.id]}
                      content={tab.content}
                      name={tab.name}
                      path={tab.scratch || tab.external ? '' : tab.path}
                      readonly={!!tab.external}
                      ongoline={() => showModal('line', String(cursor.line))}
                      onstatus={(status) => {
                        languageStatuses = {
                          ...languageStatuses,
                          [tab.id]: status,
                        };
                      }}
                      ondefinition={(path, line, column) =>
                        openFile({ path }, line, column, true)}
                      {fontSize}
                      {wrap}
                      {tabSize}
                      {insertSpaces}
                      onchange={(content) => updateContent(tab.id, content)}
                      oncursor={(line, column) => {
                        if (tab.id === activeId) cursor = { line, column };
                      }}
                      onsave={saveActive}
                      oncompile={runActive}
                    />{/if}
                </div>
              </div>{/each}
          </div>
          {#if split}<div
              class="editor-column split-column flex-1 min-w-0 min-h-0 flex flex-col overflow-hidden border-l border-l-[#534c31]"
            >
              <div
                class="breadcrumbs flex items-center gap-[8px] h-[34px] border-b border-b-[#ffffff03] py-0 px-[23px] text-[10px] text-[#737a86] overflow-hidden whitespace-nowrap shrink-0"
              >
                <FileIcon name={split.name} size={14} /><select
                  aria-label="Right editor file"
                  value={split.id}
                  onchange={(event) => (splitId = event.currentTarget.value)}
                  >{#each tabs.filter((tab) => !tab.diff) as tab}<option
                      value={tab.id}>{tab.name}</option
                    >{/each}</select
                ><button
                  class="icon-button"
                  aria-label="Close split editor"
                  onclick={() => (splitId = '')}><X size={13} /></button
                >
              </div>
              <div class="code-pane flex-1 min-h-0">
                <Editor
                  content={split.content}
                  name={split.name}
                  path={split.scratch || split.external ? '' : split.path}
                  readonly={!!split.external}
                  ondefinition={(path, line, column) =>
                    openFile({ path }, line, column, true)}
                  {fontSize}
                  {wrap}
                  {tabSize}
                  {insertSpaces}
                  onchange={(content) => updateContent(split.id, content)}
                  oncursor={() => {}}
                  onsave={() => saveTab(split)}
                  oncompile={runActive}
                />
              </div>
            </div>{/if}
        </div>
      </div>
      {#if panelVisible}<section
          class="output-panel integrated-panel h-[192px] min-h-[120px] shrink-0 flex flex-col border-t border-t-[#46402b] bg-[#101113] relative max-h-[calc(100vh_-_220px)]"
          style:height={`${panelHeight}px`}
        >
          <button
            class="panel-resizer absolute top-[-3px] h-[6px] w-full cursor-row-resize z-[2]"
            type="button"
            aria-label="Resize bottom panel"
            tabindex="0"
            onpointerdown={(event) => resize(event, 'panel')}
            onkeydown={(event) => {
              if (event.key === 'ArrowUp')
                panelHeight = Math.min(
                  window.innerHeight - 240,
                  panelHeight + 10,
                );
              if (event.key === 'ArrowDown')
                panelHeight = Math.max(120, panelHeight - 10);
            }}
          ></button>
          <div
            class="output-heading flex justify-between items-center min-h-[39px] py-0 px-[17px] border-b border-b-[#202227] shrink-0"
          >
            <div class="panel-tabs flex items-stretch gap-[18px] h-full">
              <button
                class:current={panel === 'terminal'}
                onclick={() => (panel = 'terminal')}
                >TERMINAL{#if running}<span>{running}</span>{/if}</button
              ><button
                class:current={panel === 'problems'}
                onclick={() => (panel = 'problems')}
                >PROBLEMS<span
                  >{Object.values(diagnostics).reduce(
                    (count, event) => count + event.diagnostics.length,
                    0,
                  )}</span
                ></button
              ><button
                class:current={panel === 'output'}
                onclick={() => (panel = 'output')}
                >BUILD OUTPUT{#if building}<span>RUNNING</span>{/if}</button
              >
            </div>
            <div class="output-actions">
              {#if panel === 'terminal'}<select
                  aria-label="Terminal shell"
                  bind:value={shell}
                  >{#each shells as profile}<option value={profile.path}
                      >{profile.name}</option
                    >{/each}{#if !shells.length}<option value=""
                      >Default shell</option
                    >{/if}</select
                ><button
                  class="icon-button"
                  title="New terminal"
                  aria-label="New terminal"
                  onclick={newTerminal}><Plus size={15} /></button
                >{:else if building}<button
                  class="icon-button"
                  title="Stop build"
                  onclick={() => backend().CancelBuild()}
                  ><Square size={13} /></button
                >{:else}<button
                  class="icon-button"
                  title="Clear output"
                  onclick={() => (output = '')}><Trash2 size={13} /></button
                >{/if}<button
                class="icon-button"
                title="Hide panel"
                aria-label="Hide panel"
                onclick={() => (panelVisible = false)}><X size={14} /></button
              >
            </div>
          </div>
          <div
            class="panel-content flex-1 min-h-0 flex flex-col"
            class:hidden={panel !== 'terminal'}
          >
            {#if terminals.length}<div
                class="terminal-tabs flex overflow-x-auto shrink-0 min-h-[30px] border-b border-b-[#23262d]"
              >
                {#each terminals as info}<div
                    class:current={info.id === terminalId}
                  >
                    <button
                      class="terminal-tab-name flex items-center gap-[7px] text-[10px] max-w-[230px] text-[#949faf] py-[5px] px-[4px]"
                      onclick={() => (terminalId = info.id)}
                      title={`${info.cwd}\nPID ${info.pid}`}
                      ><Terminal size={12} /><span>{info.title}</span
                      >{#if info.exited}<small
                          class:failed={info.exitCode !== 0}
                          >exit {info.exitCode}</small
                        >{:else}<span
                          class="status-dot inline-block w-[5px] h-[5px] rounded-[50%] bg-[#92b58a] shrink-0"
                        ></span>{/if}</button
                    ><button
                      class="icon-button"
                      title="Stop process"
                      disabled={info.exited}
                      onclick={() =>
                        backend().TerminalStop(info.id).catch(report)}
                      ><Square size={10} /></button
                    ><button
                      class="icon-button"
                      title="Close terminal"
                      aria-label={`Close terminal ${info.title}`}
                      onclick={() => closeTerminal(info)}
                      ><X size={12} /></button
                    >
                  </div>{/each}
              </div>
              <div class="terminal-body flex-1 min-h-0">
                {#each terminals as info (info.id)}<TerminalView
                    id={info.id}
                    visible={panel === 'terminal' &&
                      info.id === terminalId &&
                      panelVisible}
                    fontSize={terminalFontSize}
                    onexit={(code) =>
                      (terminals = terminals.map((item) =>
                        item.id === info.id
                          ? { ...item, exited: true, exitCode: code }
                          : item,
                      ))}
                    onerror={report}
                  />{/each}
              </div>{:else}<div
                class="terminal-empty flex items-center justify-center gap-[14px] h-full text-[#6f7a8b] text-[11px] p-[15px]"
              >
                <Terminal size={22} /><span
                  >{desktop
                    ? 'Start a shell in your workspace.'
                    : 'Open the desktop app to connect to your computer’s shell.'}</span
                ><button
                  class="secondary-button"
                  disabled={!desktop}
                  onclick={newTerminal}>New terminal</button
                >
              </div>{/if}
          </div>
          {#if panel === 'problems'}<div
              class="problems-list py-[10px] px-[18px] overflow-auto flex-1"
            >
              {#each Object.values(diagnostics) as event}{#each event.diagnostics as problem}<button
                    onclick={() =>
                      openFile(
                        { path: event.path },
                        problem.range.start.line + 1,
                        problem.range.start.character + 1,
                        true,
                      )}
                    ><CircleAlert size={13} /><span>{problem.message}</span
                    ><small
                      >{relative(event.path)}:{problem.range.start.line +
                        1}</small
                    ></button
                  >{/each}{/each}{#if !Object.values(diagnostics).some((event) => event.diagnostics.length)}<p
                  class="panel-caption"
                >
                  No reported problems. Language diagnostics require an
                  installed language server.
                </p>{/if}
            </div>{/if}
          {#if panel === 'output'}{#if output}<pre
                class="build-log whitespace-pre-wrap wrap-anywhere font-mono text-[11px] leading-[1.8] text-[#b0b8c7] py-[14px] px-[22px] m-0 flex-1 overflow-auto"
                bind:this={consoleElement}>{output}</pre>{:else}<div
                class="output-empty flex gap-[11px] items-start py-[24px] px-[23px] text-[#656d7a] text-[11px]"
              >
                <span
                  class="output-prompt text-[#b29e47] [font-family:monospace] font-bold"
                  >›</span
                ><span
                  >Build output appears here. Project tasks run in their own
                  terminals.</span
                >
              </div>{/if}{/if}
        </section>{/if}
    </main>
    {#if agentVisible}<button
        class="agent-resizer w-[4px] shrink-0 cursor-col-resize p-0 bg-[#141517] touch-none"
        aria-label="Resize agent chat"
        title={`Agent panel: ${agentWidth}px. Drag or use the left and right arrow keys to resize.`}
        onpointerdown={(event) => resize(event, 'agent')}
        onkeydown={(event) => {
          if (event.key === 'ArrowLeft' || event.key === 'ArrowRight') {
            event.preventDefault();
            agentWidth = Math.max(
              280,
              Math.min(
                520,
                agentWidth + (event.key === 'ArrowLeft' ? 10 : -10),
              ),
            );
          }
        }}
      ></button>{/if}
    <aside
      class="agent-dock shrink-0 min-w-[280px] max-w-[520px] min-h-0 border-l border-l-line"
      class:hidden={!agentVisible}
      style:width={`${agentWidth}px`}
      aria-label="Agent chat panel"
    >
      <AgentPanel
        workspace={project?.path ?? ''}
        workspaceName={project?.name ?? ''}
        files={chatFiles}
        activeFile={active && !active.diff && !active.external
          ? { name: active.name, path: active.path || `untitled:${active.id}` }
          : null}
        visible={agentVisible}
        onclose={() => (agentVisible = false)}
        onreadfile={readChatFile}
      />
    </aside>
  </div>
  <footer
    class="statusbar flex justify-between items-center h-[27px] border-t border-t-[#39362a] bg-[#171811] shrink-0 text-[9px] text-[#868877]"
  >
    <div class="status-left flex items-center gap-[7px] h-full">
      <span
        class="status-remote flex justify-center items-center w-[37px] bg-[#c8ae45] h-full text-[#282515] mr-[5px]"
        ><Code2 size={13} /></span
      ><button onclick={() => showActivity('git')}
        ><GitBranch size={11} />{git?.branch || 'Local workspace'}</button
      >{#if running}<span>· {running} running task{running > 1 ? 's' : ''}</span
        >{/if}{#if !desktop}<span
          class="preview-note text-[#676c5c] ml-[12px] text-[9px]"
          >Editor preview</span
        >{/if}
    </div>
    <div class="status-right flex items-center gap-[19px] pr-[13px]">
      <button onclick={() => showModal('line', String(cursor.line))}
        >Ln {cursor.line}, Col {cursor.column}</button
      ><button onclick={() => showModal('settings')}
        >{insertSpaces ? 'Spaces' : 'Tab size'}: {tabSize}</button
      ><span>UTF-8</span><span
        title={languageStatus?.problem || languageStatus?.server || ''}
        >{active
          ? language(active.name)
          : 'Praewa Code'}{languageStatus?.available ? ' · LSP' : ''}</span
      ><button onclick={() => (wrap = !wrap)} title="Word wrap"
        ><WrapText size={12} /></button
      >
    </div>
  </footer>
</div>
{#if explorerMenu}<ContextMenu
    x={explorerMenu.x}
    y={explorerMenu.y}
    items={contextItems}
    onclose={() => (explorerMenu = null)}
    onerror={report}
  />{/if}
{#if notice}<div class="toast" role="status">
    <CircleAlert size={16} /><span>{notice}</span>{#if undoTrash}<button
        onclick={restoreTrash}>Undo trash</button
      >{/if}<button
      class="icon-button"
      aria-label="Dismiss notification"
      onclick={() => (notice = '')}><X size={14} /></button
    >
  </div>{/if}
{#if modal}<dialog
    class="modal"
    class:command-modal={modal === 'commands' ||
      modal === 'quick' ||
      modal === 'task'}
    use:mountDialog
    oncancel={(event) => {
      event.preventDefault();
      closeModal();
    }}
  >
    {#if modal === 'commands' || modal === 'quick' || modal === 'task'}
      <div
        class="palette-input flex items-center gap-[12px] py-[16px] px-[19px] border-b border-b-[#33352d] text-[#a79b63]"
      >
        <Search size={17} /><input
          aria-label={modal === 'quick'
            ? 'Quick open'
            : modal === 'task'
              ? 'Find task'
              : 'Search commands'}
          placeholder={modal === 'quick'
            ? 'Search files by name or path…'
            : modal === 'task'
              ? 'Choose a task…'
              : 'Type a command…'}
          bind:value={input}
          oninput={() => (selection = 0)}
          onkeydown={paletteKey}
        /><button class="icon-button" onclick={closeModal}
          ><X size={15} /></button
        >
      </div>
      <div
        class="command-list pt-0 pr-[8px] pb-[10px] pl-[8px] max-h-[390px] overflow-auto pt-[8px]"
      >
        {#if modal === 'commands'}{#each filteredCommands as command, i}<button
              class:highlighted={i === selection}
              onclick={() => {
                modal = null;
                void Promise.resolve(command.action()).catch(report);
              }}
              ><command.icon size={16} /><span>{command.label}</span><kbd
                >{command.key}</kbd
              ></button
            >{/each}
        {:else if modal === 'quick'}{#each quickFiles as file, i}<button
              class:highlighted={i === selection}
              onclick={() => {
                modal = null;
                void openFile(file);
              }}
              ><FileIcon name={file.name} /><span
                >{file.name}<small>{relative(file.path)}</small></span
              ></button
            >{/each}{#if !quickFiles.length}<p
              class="no-commands text-[#7a8392] text-[12px] text-center p-[20px]"
            >
              No files found. Open a workspace first.
            </p>{/if}
        {:else}{#each filteredTasks as task, i}<button
              class:highlighted={i === selection}
              disabled={!!task.problem}
              title={task.problem}
              onclick={() => runTask(task)}
              ><Play size={15} /><span
                >{task.label}<small
                  >{task.problem ||
                    `${task.command} ${(task.args ?? []).join(' ')}`}</small
                ></span
              ></button
            >{/each}{#if !filteredTasks.length}<p
              class="no-commands text-[#7a8392] text-[12px] text-center p-[20px]"
            >
              No tasks available in this workspace.
            </p>{/if}{/if}
      </div>
      <div
        class="palette-footer flex items-center gap-[8px] border-t border-t-[#2d2f34] py-[12px] px-[20px] text-[#686f7d] text-[10px]"
      >
        ↑ ↓ to select · Enter to open<kbd>esc</kbd>
      </div>
    {:else}
      <div class="modal-heading flex items-center justify-between gap-[20px]">
        <h2>
          {modal === 'confirm'
            ? 'Confirm action'
            : modal === 'settings'
              ? 'Workspace settings'
              : modal === 'saveas'
                ? 'Save as'
                : modal === 'line'
                  ? 'Go to line'
                  : modal === 'command'
                    ? 'Run command'
                    : 'New file'}
        </h2>
        <button
          class="icon-button"
          aria-label="Close dialog"
          onclick={closeModal}><X size={16} /></button
        >
      </div>
      {#if modal === 'confirm'}<p
          class="modal-description text-[12px] leading-[1.8] text-[#8d939f] mt-[16px] mr-0 mb-[23px] ml-0"
        >
          {confirmText}
        </p>
        <div
          class="modal-footer flex items-center justify-end gap-[10px] mt-[24px]"
        >
          <button class="secondary-button" onclick={closeModal}>Cancel</button
          ><button
            class="danger-button bg-[#ed9b8110] text-[#eca88f] border border-[#784a3c]"
            onclick={confirmAction}>Continue</button
          >
        </div>
      {:else if modal === 'settings'}
        <div
          class="setting-row flex items-center justify-between py-[16px] px-0 border-t border-t-[#2e3036] text-[12px]"
        >
          <span>Editor font size</span>
          <div
            class="stepper flex items-center gap-[15px] border border-[#35383f] rounded-[5px] p-[5px]"
          >
            <button
              class="icon-button"
              onclick={() => (fontSize = Math.max(11, fontSize - 1))}
              ><Minus size={14} /></button
            ><span>{fontSize}px</span><button
              class="icon-button"
              onclick={() => (fontSize = Math.min(24, fontSize + 1))}
              ><Plus size={14} /></button
            >
          </div>
        </div>
        <div
          class="setting-row flex items-center justify-between py-[16px] px-0 border-t border-t-[#2e3036] text-[12px]"
        >
          <span>Terminal font size</span>
          <div
            class="stepper flex items-center gap-[15px] border border-[#35383f] rounded-[5px] p-[5px]"
          >
            <button
              class="icon-button"
              onclick={() =>
                (terminalFontSize = Math.max(10, terminalFontSize - 1))}
              ><Minus size={14} /></button
            ><span>{terminalFontSize}px</span><button
              class="icon-button"
              onclick={() =>
                (terminalFontSize = Math.min(22, terminalFontSize + 1))}
              ><Plus size={14} /></button
            >
          </div>
        </div>
        <div
          class="setting-row flex items-center justify-between py-[16px] px-0 border-t border-t-[#2e3036] text-[12px]"
        >
          <span>Word wrap</span><button
            class="toggle w-[34px] h-[19px] rounded-[20px] bg-[#353942] flex items-center p-[3px]"
            class:enabled={wrap}
            aria-label="Word wrap"
            aria-pressed={wrap}
            onclick={() => (wrap = !wrap)}><span></span></button
          >
        </div>
        <div
          class="setting-row flex items-center justify-between py-[16px] px-0 border-t border-t-[#2e3036] text-[12px]"
        >
          <span>Insert spaces</span><button
            class="toggle w-[34px] h-[19px] rounded-[20px] bg-[#353942] flex items-center p-[3px]"
            class:enabled={insertSpaces}
            aria-label="Insert spaces"
            aria-pressed={insertSpaces}
            onclick={() => (insertSpaces = !insertSpaces)}><span></span></button
          >
        </div>
        <div
          class="setting-row flex items-center justify-between py-[16px] px-0 border-t border-t-[#2e3036] text-[12px]"
        >
          <span>Indent size</span><select
            aria-label="Indent size"
            bind:value={tabSize}
            ><option value={2}>2</option><option value={4}>4</option><option
              value={8}>8</option
            ></select
          >
        </div>
        <div
          class="modal-footer flex items-center justify-end gap-[10px] mt-[24px]"
        >
          <button class="primary-button" onclick={closeModal}>Done</button>
        </div>
      {:else}<form
          onsubmit={(event) => {
            event.preventDefault();
            if (modal === 'line') {
              const [line, column] = input.split(':').map(Number);
              if (Number.isFinite(line) && line > 0 && active) {
                editors[active.id]?.goTo(line, column || 1);
                modal = null;
              }
            } else if (modal === 'command') void runCommand();
            else void submitFile();
          }}
        >
          <p
            class="modal-description text-[12px] leading-[1.8] text-[#8d939f] mt-[16px] mr-0 mb-[23px] ml-0"
          >
            {modal === 'line'
              ? 'Enter line number or line:column.'
              : modal === 'command'
                ? `Run a shell command in ${project?.path ?? 'your workspace'}.`
                : `Relative to ${newParent || 'a new unsaved buffer'}. Parent directories must exist.`}
          </p>
          <input
            class="text-input w-full h-[42px] border border-[#4b4733] bg-[#0f1012] rounded-[5px] py-[10px] px-[12px] text-[#dddfe5] font-mono text-[12px]"
            aria-label={modal === 'command'
              ? 'Shell command'
              : modal === 'line'
                ? 'Line number'
                : 'File name'}
            bind:value={input}
            autocomplete="off"
          />{#if modalError}<p class="form-error">{modalError}</p>{/if}
          <div
            class="modal-footer flex items-center justify-end gap-[10px] mt-[24px]"
          >
            <button type="button" class="secondary-button" onclick={closeModal}
              >Cancel</button
            ><button type="submit" class="primary-button" disabled={working}
              >{working
                ? 'Working…'
                : modal === 'command'
                  ? 'Run'
                  : modal === 'line'
                    ? 'Go'
                    : modal === 'saveas'
                      ? 'Save'
                      : 'Create'}</button
            >
          </div>
        </form>{/if}
    {/if}
  </dialog>{/if}
