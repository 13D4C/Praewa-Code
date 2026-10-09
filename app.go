package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"

	"praewa-code/internal/agent"
	"praewa-code/internal/ide"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx       context.Context
	workspace *ide.Workspace
	builder   *ide.Builder
	terminals *ide.Terminals
	watcher   *ide.Watcher
	sessions  *ide.SessionStore
	languages *ide.Languages
	dirty     atomic.Bool
	agent     *agent.Service
}

type InitialState struct {
	Project *ide.Project       `json:"project"`
	Session ide.Session        `json:"session"`
	Shells  []ide.ShellProfile `json:"shells"`
}

func NewApp() *App {
	w := ide.NewWorkspace()
	return &App{workspace: w, builder: ide.NewBuilder(w), terminals: ide.NewTerminals(), watcher: &ide.Watcher{}, sessions: ide.NewSessionStore(), languages: ide.NewLanguages(w), agent: agent.New(w.Root)}
}

func (a *App) startup(ctx context.Context) {
	prepareCamera()
	a.ctx = ctx
	a.languages.OnDiagnostics = func(event ide.LanguageDiagnostics) { runtime.EventsEmit(ctx, "language:diagnostics", event) }
	a.builder.OnOutput = func(text string) { runtime.EventsEmit(ctx, "build:output", text) }
	a.terminals.OnEvent = func(event ide.TerminalEvent) { runtime.EventsEmit(ctx, "terminal:event", event) }
	a.watcher.OnChange = func(path string) { runtime.EventsEmit(ctx, "workspace:changed", path) }
	a.agent.OnEvent = func(event agent.Event) { runtime.EventsEmit(ctx, "agent:event", event) }
}

func (a *App) beforeClose(ctx context.Context) bool {
	if !a.dirty.Load() {
		return false
	}
	answer, err := runtime.MessageDialog(ctx, runtime.MessageDialogOptions{Type: runtime.QuestionDialog, Title: "Unsaved changes", Message: "There are unsaved changes. Close Praewa Code without saving? Available drafts can be restored next time.", Buttons: []string{"Yes", "No"}, DefaultButton: "No", CancelButton: "No"})
	return err != nil || answer != "Yes"
}

func (a *App) shutdown(context.Context) {
	a.CameraAccess(false)
	a.builder.Cancel()
	a.languages.Close()
	a.terminals.CloseAll()
	a.watcher.Close()
	a.agent.Close()
}
func (a *App) SetDirty(dirty bool) { a.dirty.Store(dirty) }

func (a *App) InitialState() (*InitialState, error) {
	session := a.sessions.Load()
	state := &InitialState{Session: session, Shells: ide.ShellProfiles()}
	if session.Workspace != "" {
		state.Project, _ = a.workspace.Open(session.Workspace)
		if state.Project != nil {
			_ = a.watcher.Start(state.Project.Path)
		}
	}
	return state, nil
}

func (a *App) openWorkspace(path string) (*ide.Project, error) {
	if a.agent.Active() {
		return nil, fmt.Errorf("Stop the current Agent response before opening another workspace")
	}
	if a.builder.Running() {
		return nil, fmt.Errorf("Stop the current build before opening another workspace")
	}
	a.languages.Close()
	project, err := a.workspace.Open(path)
	if err != nil {
		return nil, err
	}
	_ = a.watcher.Start(project.Path)
	_ = a.sessions.Save(ide.Session{Workspace: project.Path, OpenFiles: []string{}})
	return project, nil
}

func (a *App) OpenWorkspace(path string) (*ide.Project, error) { return a.openWorkspace(path) }
func (a *App) OpenFolder() (*ide.Project, error) {
	path, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{Title: "Open workspace"})
	if err != nil || path == "" {
		return nil, err
	}
	return a.openWorkspace(path)
}

func (a *App) OpenFileDialog() (*ide.Document, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Title: "Open file", DefaultDirectory: a.workspace.Root()})
	if err != nil || path == "" {
		return nil, err
	}
	return a.workspace.Read(path)
}

func (a *App) CreatePlayground() (*ide.Project, error) {
	path, err := os.MkdirTemp("", "praewa-playground-")
	if err != nil {
		return nil, err
	}
	files := map[string]string{"go.mod": "module playground\n\ngo 1.23.0\n", "main.go": "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"Hello from Praewa Code!\")\n}\n"}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(path, name), []byte(content), 0644); err != nil {
			return nil, err
		}
	}
	return a.openWorkspace(path)
}

func (a *App) SaveSession(files []string, active string) error {
	return a.sessions.Save(ide.Session{Workspace: a.workspace.Root(), OpenFiles: files, ActiveFile: active})
}
func (a *App) ListDirectory(path string) ([]ide.Entry, error) { return a.workspace.List(path) }
func (a *App) FileIndex() ([]ide.Entry, error)                { return a.workspace.FileIndex() }
func (a *App) ReadFile(path string) (*ide.Document, error)    { return a.workspace.Read(path) }
func (a *App) SaveFile(path, content string) error            { return a.workspace.Save(path, content) }
func (a *App) SaveChecked(path, content, version string) (*ide.Document, error) {
	document, err := a.workspace.SaveChecked(path, content, version)
	if err == nil {
		a.languages.Saved(document.Path)
	}
	return document, err
}
func (a *App) CreateFile(path, content string) (*ide.Document, error) {
	return a.workspace.Create(path, content)
}
func (a *App) CreateDirectory(path string) (*ide.Entry, error) {
	return a.workspace.CreateDirectory(path)
}
func (a *App) Rename(path, destination string) (*ide.Entry, error) {
	return a.workspace.Rename(path, destination)
}
func (a *App) Trash(path string) (*ide.TrashedFile, error) { return a.workspace.Trash(path) }
func (a *App) RestoreTrash(item ide.TrashedFile) error     { return a.workspace.RestoreTrash(item) }
func (a *App) Search(query ide.SearchQuery) (*ide.SearchResult, error) {
	return a.workspace.Search(query)
}
func (a *App) ReplaceSearchFile(query ide.SearchQuery, path, replacement, version string) (*ide.Document, error) {
	return a.workspace.ReplaceSearchFile(query, path, replacement, version)
}
func (a *App) Format(path, content string) (string, error) { return a.workspace.Format(path, content) }
func (a *App) Tasks(file string) ([]ide.Task, error)       { return a.workspace.Tasks(file) }
func (a *App) RunTask(id, file string) (*ide.TerminalInfo, error) {
	task, err := a.workspace.ResolveTask(id, file)
	if err != nil {
		return nil, err
	}
	return a.terminals.StartTask(*task)
}
func (a *App) RunCommand(command string) (*ide.TerminalInfo, error) {
	if a.workspace.Root() == "" {
		return nil, fmt.Errorf("Open a workspace first")
	}
	return a.terminals.StartTask(ide.Task{Label: command, Type: "shell", Command: command, Cwd: a.workspace.Root()})
}
func (a *App) NewTerminal(request ide.TerminalRequest) (*ide.TerminalInfo, error) {
	if request.Cwd == "" {
		request.Cwd = a.workspace.Root()
	}
	return a.terminals.Create(request)
}
func (a *App) TerminalSnapshot(id string) (*ide.TerminalSnapshot, error) {
	return a.terminals.Snapshot(id)
}
func (a *App) TerminalWrite(id, data string) error { return a.terminals.Write(id, data) }
func (a *App) TerminalResize(id string, columns, rows int) error {
	return a.terminals.Resize(id, columns, rows)
}
func (a *App) TerminalClose(id string) error           { return a.terminals.Close(id) }
func (a *App) TerminalStop(id string) error            { return a.terminals.Stop(id) }
func (a *App) GitStatus() *ide.GitState                { return a.workspace.GitStatus() }
func (a *App) GitStage(path string, staged bool) error { return a.workspace.GitStage(path, staged) }
func (a *App) GitDiff(path string, staged bool) (*ide.Diff, error) {
	return a.workspace.GitDiff(path, staged)
}
func (a *App) GitCommit(message string) (string, error)    { return a.workspace.GitCommit(message) }
func (a *App) GitInit() error                              { return a.workspace.GitInit() }
func (a *App) Build(path string) (*ide.BuildResult, error) { return a.builder.Build(a.ctx, path) }
func (a *App) CancelBuild()                                { a.builder.Cancel() }

func (a *App) LanguageSync(path, content string) ide.LanguageStatus {
	return a.languages.Sync(path, content)
}
func (a *App) LanguageRequest(path, content, method string, line, character int) (json.RawMessage, error) {
	return a.languages.Request(path, content, method, line, character)
}
func (a *App) LanguageClose(path string) { a.languages.CloseDocument(path) }

func (a *App) ReadSource(path string) (*ide.Document, error) { return ide.ReadText(path) }

type ChosenFile struct {
	Path   string `json:"path"`
	Parent string `json:"parent"`
}

func (a *App) ChooseFile() (*ChosenFile, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{Title: "Open file", DefaultDirectory: a.workspace.Root()})
	if err != nil || path == "" {
		return nil, err
	}
	return &ChosenFile{Path: path, Parent: filepath.Dir(path)}, nil
}
