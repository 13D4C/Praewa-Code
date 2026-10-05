<script lang="ts">
  import { onMount } from 'svelte';
  import { basicSetup } from 'codemirror';
  import {
    undo as undoCommand,
    redo as redoCommand,
    indentWithTab,
  } from '@codemirror/commands';
  import { EditorState, Compartment, type Extension } from '@codemirror/state';
  import { EditorView, keymap, hoverTooltip } from '@codemirror/view';
  import {
    indentUnit,
    syntaxHighlighting,
    HighlightStyle,
  } from '@codemirror/language';
  import { openSearchPanel, gotoLine } from '@codemirror/search';
  import { tags } from '@lezer/highlight';
  import { autocompletion, completeFromList } from '@codemirror/autocomplete';
  import { linter, setDiagnostics, lintGutter } from '@codemirror/lint';
  import { go } from '@codemirror/lang-go';
  import { javascript } from '@codemirror/lang-javascript';
  import { json } from '@codemirror/lang-json';
  import { python } from '@codemirror/lang-python';
  import { rust } from '@codemirror/lang-rust';
  import { cpp } from '@codemirror/lang-cpp';
  import { css } from '@codemirror/lang-css';
  import { html } from '@codemirror/lang-html';
  import { markdown } from '@codemirror/lang-markdown';
  import { backend, isDesktop, onEvent } from '../lib/bridge';
  import {
    offset,
    uriPath,
    type DiagnosticsEvent,
    type LanguageStatus,
    type CompletionItem,
  } from '../lib/language';

  let {
    content,
    name,
    path = '',
    readonly = false,
    ongoline,
    onstatus = () => {},
    ondefinition = () => {},
    fontSize = 14,
    wrap = false,
    tabSize = 4,
    insertSpaces = false,
    onchange,
    oncursor,
    onsave,
    oncompile,
  }: {
    content: string;
    name: string;
    path?: string;
    readonly?: boolean;
    ongoline?: () => void;
    onstatus?: (status: LanguageStatus) => void;
    ondefinition?: (path: string, line: number, column: number) => void;
    fontSize?: number;
    wrap?: boolean;
    tabSize?: number;
    insertSpaces?: boolean;
    onchange: (value: string) => void;
    oncursor: (line: number, column: number) => void;
    onsave: () => void;
    oncompile: () => void;
  } = $props();
  let host: HTMLDivElement;
  let view = $state.raw<EditorView>();
  const indentation = new Compartment();
  const font = new Compartment();
  const wrapping = new Compartment();
  const languageMode = new Compartment();
  const attributes = new Compartment();
  const editing = new Compartment();
  let languageVersion = 0;
  let syncedText = '';
  let disposed = false;
  const languageFeatures = new Compartment();
  async function request(method: string, position: number) {
    if (!view || !path || !isDesktop()) return null;
    const text = view.state.doc.toString();
    const line = view.state.doc.lineAt(position);
    return backend()
      .LanguageRequest(
        path,
        text,
        method,
        line.number - 1,
        position - line.from,
      )
      .catch(() => null);
  }
  async function definition() {
    if (!view) return;
    const result = await request('definition', view.state.selection.main.head);
    const item = Array.isArray(result) ? result[0] : result;
    if (item) {
      const range = item.targetSelectionRange ?? item.range;
      const path = uriPath(item.targetUri ?? item.uri);
      if (path && range)
        ondefinition(path, range.start.line + 1, range.start.character + 1);
    }
  }
  async function completions(
    context: import('@codemirror/autocomplete').CompletionContext,
  ) {
    const word = context.matchBefore(/[\w$]*/);
    if (!word || (!context.explicit && word.from === word.to)) return null;
    const raw = await request('completion', context.pos);
    if (context.aborted) return null;
    const items: CompletionItem[] = Array.isArray(raw)
      ? raw
      : (raw?.items ?? []);
    const options = items
      .filter((item) => item.insertTextFormat !== 2)
      .map((item) => ({
        label: item.label,
        detail: item.detail,
        type:
          (
            {
              2: 'function',
              3: 'function',
              5: 'variable',
              6: 'variable',
              7: 'class',
              9: 'namespace',
              10: 'property',
              14: 'keyword',
              21: 'constant',
            } as Record<number, string>
          )[item.kind ?? 0] ?? 'text',
        apply(
          editor: EditorView,
          _completion: unknown,
          from: number,
          to: number,
        ) {
          const text = editor.state.doc.toString();
          const range =
            item.textEdit?.range ??
            item.textEdit?.replace ??
            item.textEdit?.insert;
          const insert =
            item.textEdit?.newText ?? item.insertText ?? item.label;
          const changes = [
            {
              from: range ? offset(text, range.start) : from,
              to: range ? offset(text, range.end) : to,
              insert,
            },
            ...(item.additionalTextEdits ?? []).map((edit) => ({
              from: offset(text, edit.range.start),
              to: offset(text, edit.range.end),
              insert: edit.newText,
            })),
          ].sort((a, b) => a.from - b.from);
          editor.dispatch({ changes });
        },
      }));
    if (!options.length) {
      const words = [
        ...new Set(
          context.state.doc.toString().match(/[a-zA-Z_$][\w$]{2,}/g) ?? [],
        ),
      ];
      return completeFromList(
        words.map((label) => ({ label, type: 'variable' })),
      )(context);
    }
    return { from: word.from, options, validFor: /^[\w$]*$/ };
  }
  const hover = hoverTooltip(async (editor, pos) => {
    const text = editor.state.doc.toString();
    const result = await request('hover', pos);
    if (!result?.contents || text !== editor.state.doc.toString()) return null;
    const contents = result.contents;
    const value =
      typeof contents === 'string'
        ? contents
        : Array.isArray(contents)
          ? contents
              .map((item) => (typeof item === 'string' ? item : item.value))
              .join('\n')
          : contents.value;
    return {
      pos,
      end: pos,
      above: true,
      create() {
        const dom = document.createElement('pre');
        dom.className = 'language-hover';
        dom.textContent = String(value);
        return { dom };
      },
    };
  });

  function syntax(): Extension {
    const ext = name.split('.').pop()?.toLowerCase();
    switch (ext) {
      case 'go':
        return go();
      case 'ts':
        return javascript({ typescript: true });
      case 'tsx':
        return javascript({ typescript: true, jsx: true });
      case 'jsx':
        return javascript({ jsx: true });
      case 'js':
      case 'mjs':
      case 'cjs':
        return javascript();
      case 'json':
        return json();
      case 'py':
        return python();
      case 'rs':
        return rust();
      case 'c':
      case 'h':
      case 'cpp':
      case 'hpp':
      case 'cc':
      case 'cxx':
        return cpp();
      case 'css':
        return css();
      case 'html':
      case 'svelte':
        return html();
      case 'md':
        return markdown();
      default:
        return [];
    }
  }

  const theme = EditorView.theme(
    {
      '&': { height: '100%', color: '#cdd0d8', backgroundColor: '#111214' },
      '.cm-scroller': {
        fontFamily: '"JetBrains Mono", "SFMono-Regular", Consolas, monospace',
        lineHeight: '1.85',
        overflow: 'auto',
      },
      '.cm-content': { padding: '20px 0', caretColor: '#f5d547' },
      '.cm-line': { padding: '0 26px 0 16px' },
      '.cm-gutters': {
        backgroundColor: '#111214',
        color: '#494d55',
        border: 'none',
        padding: '20px 0 0 18px',
      },
      '.cm-lineNumbers .cm-gutterElement': {
        minWidth: '36px',
        paddingRight: '16px',
      },
      '.cm-activeLineGutter': {
        color: '#f5d547',
        backgroundColor: 'transparent',
      },
      '.cm-activeLine': { backgroundColor: '#ffffff03' },
      '&.cm-focused .cm-selectionBackground, .cm-selectionBackground, ::selection':
        { backgroundColor: '#f5d54725' },
      '&.cm-focused': { outline: 'none' },
      '.cm-cursor': { borderLeftColor: '#f5d547' },
      '.cm-foldPlaceholder': {
        backgroundColor: '#23252a',
        borderColor: '#393a3e',
        color: '#d9c65b',
      },
      '.cm-panels': { backgroundColor: '#191a1e', color: '#cdd0d8' },
      '.cm-panels-top': { borderBottom: '1px solid #303239' },
      '.cm-search input, .cm-search button': {
        color: '#d9dce3',
        backgroundColor: '#24262c',
        border: '1px solid #393b43',
        borderRadius: '4px',
      },
      '.cm-tooltip': {
        backgroundColor: '#1d1f24',
        border: '1px solid #353840',
        borderRadius: '6px',
      },
      '.cm-tooltip-autocomplete ul li[aria-selected]': {
        backgroundColor: '#f5d54725',
        color: '#f5d547',
      },
      '.cm-matchingBracket': { color: '#f5d547', backgroundColor: '#f5d54718' },
    },
    { dark: true },
  );

  const highlighting = HighlightStyle.define([
    { tag: [tags.keyword, tags.modifier], color: '#d6b6fb' },
    { tag: [tags.string, tags.special(tags.string)], color: '#e2cb80' },
    { tag: [tags.number, tags.bool, tags.null], color: '#e69c75' },
    {
      tag: [tags.function(tags.variableName), tags.function(tags.propertyName)],
      color: '#82c5e7',
    },
    { tag: [tags.typeName, tags.className], color: '#a5d6bd' },
    { tag: [tags.comment], color: '#626874', fontStyle: 'italic' },
    { tag: [tags.operator, tags.punctuation], color: '#9097a5' },
    { tag: [tags.propertyName, tags.attributeName], color: '#b5c4e4' },
    { tag: tags.heading, color: '#f5d547', fontWeight: 'bold' },
  ]);

  onMount(() => {
    view = new EditorView({
      parent: host,
      state: EditorState.create({
        doc: content,
        extensions: [
          keymap.of([
            indentWithTab,
            {
              key: 'Mod-g',
              run: (editor) => {
                if (ongoline) ongoline();
                else gotoLine(editor);
                return true;
              },
            },
            {
              key: 'Mod-s',
              run: () => {
                onsave();
                return true;
              },
            },
            {
              key: 'Mod-Enter',
              run: () => {
                oncompile();
                return true;
              },
            },
            {
              key: 'F12',
              run: () => {
                void definition();
                return true;
              },
            },
          ]),
          basicSetup,
          editing.of([
            EditorState.readOnly.of(readonly),
            EditorView.editable.of(!readonly),
          ]),
          theme,
          languageMode.of(syntax()),
          syntaxHighlighting(highlighting),
          indentation.of([
            indentUnit.of(insertSpaces ? ' '.repeat(tabSize) : '\t'),
            EditorState.tabSize.of(tabSize),
          ]),
          languageFeatures.of([
            autocompletion({ override: [completions] }),
            hover,
            lintGutter(),
          ]),
          linter((state) => {
            if (!name.endsWith('.json')) return [];
            try {
              JSON.parse(state.state.doc.toString());
              return [];
            } catch (error) {
              const message = String(error);
              const position = Number(
                message.match(/position (\d+)/)?.[1] ?? 0,
              );
              return [
                {
                  from: Math.min(position, state.state.doc.length),
                  to: Math.min(position + 1, state.state.doc.length),
                  severity: 'error',
                  message,
                },
              ];
            }
          }),
          font.of(
            EditorView.theme({ '.cm-scroller': { fontSize: `${fontSize}px` } }),
          ),
          wrapping.of(wrap ? EditorView.lineWrapping : []),
          attributes.of(
            EditorView.contentAttributes.of({
              'aria-label': `Code editor: ${name}`,
            }),
          ),
          EditorView.updateListener.of((update) => {
            if (update.docChanged) onchange(update.state.doc.toString());
            if (update.selectionSet || update.docChanged) {
              const position = update.state.selection.main.head;
              const line = update.state.doc.lineAt(position);
              oncursor(line.number, position - line.from + 1);
            }
          }),
        ],
      }),
    });
    const unsubscribe = onEvent<DiagnosticsEvent>(
      'language:diagnostics',
      (event) => {
        if (
          !view ||
          event.path !== path ||
          view.state.doc.toString() !== syncedText ||
          (event.version && event.version < languageVersion)
        )
          return;
        const text = view.state.doc.toString();
        view.dispatch(
          setDiagnostics(
            view.state,
            event.diagnostics.map((item) => ({
              from: offset(text, item.range.start),
              to: offset(text, item.range.end),
              severity:
                item.severity === 1
                  ? 'error'
                  : item.severity === 2
                    ? 'warning'
                    : 'info',
              message: item.message,
              source: item.source,
            })),
          ),
        );
      },
    );
    return () => {
      disposed = true;
      unsubscribe();
      view?.destroy();
      view = undefined;
    };
  });

  $effect(() => {
    const size = fontSize;
    const shouldWrap = wrap;
    const locked = readonly;
    const mode = syntax();
    const indent = insertSpaces ? ' '.repeat(tabSize) : '\t';
    const sizeTab = tabSize;
    view?.dispatch({
      effects: [
        editing.reconfigure([
          EditorState.readOnly.of(locked),
          EditorView.editable.of(!locked),
        ]),
        font.reconfigure(
          EditorView.theme({ '.cm-scroller': { fontSize: `${size}px` } }),
        ),
        wrapping.reconfigure(shouldWrap ? EditorView.lineWrapping : []),
        languageMode.reconfigure(mode),
        attributes.reconfigure(
          EditorView.contentAttributes.of({
            'aria-label': `Code editor: ${name}`,
          }),
        ),
        indentation.reconfigure([
          indentUnit.of(indent),
          EditorState.tabSize.of(sizeTab),
        ]),
      ],
    });
    view?.requestMeasure();
  });

  $effect(() => {
    const text = content;
    if (view) {
      const previous = view.state.doc.toString();
      if (previous === text) return;
      let from = 0;
      while (
        from < previous.length &&
        from < text.length &&
        previous[from] === text[from]
      )
        from++;
      let oldEnd = previous.length,
        newEnd = text.length;
      while (
        oldEnd > from &&
        newEnd > from &&
        previous[oldEnd - 1] === text[newEnd - 1]
      ) {
        oldEnd--;
        newEnd--;
      }
      view.dispatch({
        changes: { from, to: oldEnd, insert: text.slice(from, newEnd) },
      });
    }
  });
  $effect(() => {
    const text = content;
    const file = path;
    if (!file || !isDesktop()) return;
    const timer = setTimeout(() => {
      syncedText = text;
      void backend()
        .LanguageSync(file, text)
        .then((status) => {
          if (!disposed) {
            languageVersion = status.version;
            onstatus(status);
          }
        })
        .catch(() => {});
    }, 350);
    return () => clearTimeout(timer);
  });
  export function goTo(line: number, column = 1) {
    if (!view) return;
    const target = view.state.doc.line(
      Math.max(1, Math.min(line, view.state.doc.lines)),
    );
    const offset = [...target.text].slice(0, column - 1).join('').length;
    const position = Math.min(target.to, target.from + offset);
    view.dispatch({ selection: { anchor: position }, scrollIntoView: true });
    view.focus();
  }
  export function undo() {
    if (view) undoCommand(view);
  }
  export function redo() {
    if (view) redoCommand(view);
  }
  export function search() {
    if (view) {
      openSearchPanel(view);
    }
  }
  export function focus() {
    if (!view) return;
    view.focus();
    const position = view.state.selection.main.head;
    const line = view.state.doc.lineAt(position);
    oncursor(line.number, position - line.from + 1);
  }
</script>

<div class="editor-host h-full min-h-0 overflow-hidden" bind:this={host}></div>
