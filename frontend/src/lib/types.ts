export type Entry = { name: string; path: string; directory: boolean };
export type Project = { name: string; path: string; entries: Entry[] };
export type Document = {
  name: string;
  path: string;
  content: string;
  version: string;
};
export type Diff = {
  name: string;
  path: string;
  original: string;
  modified: string;
  patch: string;
};
export type Tab = Document & {
  id: string;
  saved: string;
  scratch: boolean;
  external?: boolean;
  conflict?: boolean;
  missing?: boolean;
  diff?: Diff;
};
export type BuildResult = {
  command: string;
  output: string;
  exitCode: number;
  duration: number;
  artifact: string;
  cancelled: boolean;
};
export type Task = {
  id: string;
  label: string;
  type: string;
  command: string;
  args: string[];
  cwd: string;
  source: string;
  default: boolean;
  problem: string;
};
export type ShellProfile = { name: string; path: string; default: boolean };
export type TerminalInfo = {
  id: string;
  title: string;
  cwd: string;
  shell: string;
  pid: number;
  exited: boolean;
  exitCode: number;
  task: boolean;
};
export type TerminalEvent = {
  id: string;
  sequence: number;
  data: string;
  exited: boolean;
  exitCode: number;
};
export type TerminalSnapshot = {
  info: TerminalInfo;
  chunks: TerminalEvent[];
  sequence: number;
};
export type TerminalRequest = {
  shell: string;
  cwd: string;
  columns: number;
  rows: number;
};
export type Session = {
  recent: string[];
  workspace: string;
  openFiles: string[];
  activeFile: string;
};
export type InitialState = {
  project: Project | null;
  session: Session;
  shells: ShellProfile[];
};
export type SearchQuery = {
  text: string;
  regex: boolean;
  caseSensitive: boolean;
  wholeWord: boolean;
  include: string;
};
export type SearchMatch = {
  path: string;
  line: number;
  column: number;
  text: string;
  length: number;
  version: string;
};
export type SearchResult = { matches: SearchMatch[]; truncated: boolean };
export type GitFile = {
  path: string;
  original: string;
  index: string;
  worktree: string;
};
export type GitState = {
  available: boolean;
  repository: boolean;
  branch: string;
  files: GitFile[];
  log: string;
  problem: string;
};
export type TrashedFile = { original: string; stored: string };
export function language(name: string) {
  const ext = name.split('.').pop()?.toLowerCase() ?? '';
  return (
    (
      {
        go: 'Go',
        js: 'JavaScript',
        mjs: 'JavaScript',
        cjs: 'JavaScript',
        ts: 'TypeScript',
        tsx: 'TypeScript',
        jsx: 'JavaScript',
        json: 'JSON',
        py: 'Python',
        rs: 'Rust',
        c: 'C',
        h: 'C',
        cpp: 'C++',
        cc: 'C++',
        cxx: 'C++',
        hpp: 'C++',
        css: 'CSS',
        html: 'HTML',
        svelte: 'Svelte',
        md: 'Markdown',
      } as Record<string, string>
    )[ext] ?? 'Plain text'
  );
}
export const starter =
  'package main\n\nimport "fmt"\n\nfunc main() {\n\tfmt.Println("Hello from Praewa Code!")\n}\n';
