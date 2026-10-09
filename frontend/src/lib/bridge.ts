import type {
  BuildResult,
  Document,
  Entry,
  Project,
  InitialState,
  Task,
  TerminalInfo,
  TerminalRequest,
  TerminalSnapshot,
  SearchQuery,
  SearchResult,
  GitState,
  Diff,
  TrashedFile,
} from './types';
import type { LanguageStatus } from './language';
type Backend = {
  CameraAccess(enabled: boolean): Promise<void>;
  LanguageSync(path: string, content: string): Promise<LanguageStatus>;
  LanguageRequest(
    path: string,
    content: string,
    method: string,
    line: number,
    character: number,
  ): Promise<any>;
  LanguageClose(path: string): Promise<void>;
  InitialState(): Promise<InitialState>;
  OpenFolder(): Promise<Project | null>;
  OpenWorkspace(path: string): Promise<Project>;
  OpenFileDialog(): Promise<Document | null>;
  ChooseFile(): Promise<{ path: string; parent: string } | null>;
  CreatePlayground(): Promise<Project>;
  ListDirectory(path: string): Promise<Entry[]>;
  FileIndex(): Promise<Entry[]>;
  ReadFile(path: string): Promise<Document>;
  ReadSource(path: string): Promise<Document>;
  SaveChecked(
    path: string,
    content: string,
    version: string,
  ): Promise<Document>;
  CreateFile(path: string, content: string): Promise<Document>;
  CreateDirectory(path: string): Promise<Entry>;
  Rename(path: string, destination: string): Promise<Entry>;
  Trash(path: string): Promise<TrashedFile>;
  RestoreTrash(item: TrashedFile): Promise<void>;
  SaveSession(files: string[], active: string): Promise<void>;
  Search(query: SearchQuery): Promise<SearchResult>;
  ReplaceSearchFile(
    query: SearchQuery,
    path: string,
    replacement: string,
    version: string,
  ): Promise<Document>;
  Format(path: string, content: string): Promise<string>;
  Tasks(file: string): Promise<Task[]>;
  RunTask(id: string, file: string): Promise<TerminalInfo>;
  RunCommand(command: string): Promise<TerminalInfo>;
  NewTerminal(request: TerminalRequest): Promise<TerminalInfo>;
  TerminalSnapshot(id: string): Promise<TerminalSnapshot>;
  TerminalWrite(id: string, data: string): Promise<void>;
  TerminalResize(id: string, columns: number, rows: number): Promise<void>;
  TerminalClose(id: string): Promise<void>;
  TerminalStop(id: string): Promise<void>;
  GitStatus(): Promise<GitState>;
  GitStage(path: string, staged: boolean): Promise<void>;
  GitDiff(path: string, staged: boolean): Promise<Diff>;
  GitCommit(message: string): Promise<string>;
  GitInit(): Promise<void>;
  Build(path: string): Promise<BuildResult>;
  CancelBuild(): Promise<void>;
  SetDirty(dirty: boolean): Promise<void>;
};
declare global {
  interface Window {
    go?: { main: { App: Backend } };
    runtime?: {
      EventsOn(name: string, callback: (...args: any[]) => void): () => void;
      BrowserOpenURL(url: string): void;
    };
  }
}
export function isDesktop() {
  return Boolean(window.go?.main?.App);
}
export function backend(): Backend {
  const app = window.go?.main?.App;
  if (!app)
    throw new Error(
      'Open the desktop app to use local files, Git, build tools and the real terminal.',
    );
  return app;
}
export function onEvent<T>(name: string, callback: (event: T) => void) {
  return window.runtime?.EventsOn(name, callback) ?? (() => {});
}
export function onBuildOutput(callback: (text: string) => void) {
  return onEvent('build:output', callback);
}
export function openLink(url: string) {
  if (/^https?:\/\//i.test(url)) {
    if (window.runtime) window.runtime.BrowserOpenURL(url);
    else window.open(url, '_blank', 'noopener,noreferrer');
  }
}
