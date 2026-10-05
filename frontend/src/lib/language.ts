export type Position = { line: number; character: number };
export type Range = { start: Position; end: Position };
export type TextEdit = { range: Range; newText: string };
export type DiagnosticItem = {
  range: Range;
  message: string;
  severity?: number;
  source?: string;
};
export type LanguageStatus = {
  language: string;
  server: string;
  available: boolean;
  version: number;
  problem: string;
};
export type DiagnosticsEvent = {
  path: string;
  version: number;
  diagnostics: DiagnosticItem[];
};
export type CompletionItem = {
  label: string;
  detail?: string;
  insertText?: string;
  insertTextFormat?: number;
  kind?: number;
  textEdit?: TextEdit & { insert?: Range; replace?: Range };
  additionalTextEdits?: TextEdit[];
};
export function offset(text: string, position: Position) {
  const lines = text.split('\n');
  let result = 0;
  for (let i = 0; i < Math.min(position.line, lines.length - 1); i++)
    result += lines[i].length + 1;
  return Math.min(
    text.length,
    result +
      Math.min(
        position.character,
        lines[Math.min(position.line, lines.length - 1)]?.length ?? 0,
      ),
  );
}
export function applyEdits(text: string, edits: TextEdit[]) {
  for (const edit of [...edits].sort(
    (a, b) => offset(text, b.range.start) - offset(text, a.range.start),
  )) {
    const from = offset(text, edit.range.start),
      to = offset(text, edit.range.end);
    text = text.slice(0, from) + edit.newText + text.slice(to);
  }
  return text;
}
export function uriPath(uri: string) {
  try {
    const url = new URL(uri);
    if (url.protocol !== 'file:') return '';
    let path = decodeURIComponent(url.pathname);
    if (/^\/[A-Z]:/i.test(path)) path = path.slice(1);
    return path;
  } catch {
    return '';
  }
}
