package ide

import (
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testWorkspace(t *testing.T) *Workspace {
	t.Helper()
	w := NewWorkspace()
	if _, err := w.Open(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	return w
}

func TestFileOperationsAndConflict(t *testing.T) {
	w := testWorkspace(t)
	document, err := w.Create("file.txt", "original")
	if err != nil {
		t.Fatal(err)
	}
	if document.Version == "" {
		t.Fatal("missing version")
	}
	if err := os.WriteFile(document.Path, []byte("external"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := w.SaveChecked(document.Path, "overwrite", document.Version); err == nil {
		t.Fatal("overwrote external changes")
	}
	if _, err := w.CreateDirectory("folder"); err != nil {
		t.Fatal(err)
	}
	entry, err := w.Rename(document.Path, "folder/new.txt")
	if err != nil {
		t.Fatal(err)
	}
	item, err := w.Trash(entry.Path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(entry.Path); !os.IsNotExist(err) {
		t.Fatal("trash did not move file")
	}
	if err := w.RestoreTrash(*item); err != nil {
		t.Fatal(err)
	}
	restored, err := w.Read(entry.Path)
	if err != nil || restored.Content != "external" {
		t.Fatal(restored, err)
	}
	if _, err := w.Trash(filepath.Join(w.Root(), ".praewa")); err == nil {
		t.Fatal("allowed internal directory trash")
	}
	if _, err := w.Rename(entry.Path, "../escaped.txt"); err == nil {
		t.Fatal("allowed external destination")
	}
}

func TestSearchAndReplaceVersions(t *testing.T) {
	w := testWorkspace(t)
	doc, _ := w.Create("main.txt", "hello HELLO\nother hello\n")
	query := SearchQuery{Text: "hello"}
	result, err := w.Search(query)
	if err != nil || len(result.Matches) != 3 {
		t.Fatal(result, err)
	}
	if result.Matches[2].Line != 2 || result.Matches[2].Column != 7 {
		t.Fatal(result.Matches)
	}
	updated, err := w.ReplaceSearchFile(query, doc.Path, "world", doc.Version)
	if err != nil || updated.Content != "world world\nother world\n" {
		t.Fatal(updated, err)
	}
	if _, err := w.ReplaceSearchFile(query, doc.Path, "bad", doc.Version); err == nil {
		t.Fatal("replaced stale version")
	}
	if _, err := w.Search(SearchQuery{Text: "[", Regex: true}); err == nil {
		t.Fatal("allowed invalid regex")
	}
}

func TestTasksUseWorkspaceOptionsAndRejectUnsupported(t *testing.T) {
	w := testWorkspace(t)
	_, _ = w.CreateDirectory(".vscode")
	_, err := w.Create(".vscode/tasks.json", `{
  "tasks": [
    {"label":"Local build","type":"process","command":"go","args":["build","${fileBasename}"],"options":{"cwd":"${workspaceFolder}","env":{"TEST":"${workspaceFolderBasename}"}},"group":{"kind":"build","isDefault":true}},
    {"label":"Unsupported","command":"echo ok","dependsOn":"Local build"},
  ]
}`)
	if err != nil {
		t.Fatal(err)
	}
	doc, _ := w.Create("main.go", "package main\nfunc main() {}\n")
	tasks, err := w.Tasks(doc.Path)
	if err != nil || len(tasks) != 3 {
		t.Fatal(tasks, err)
	}
	if tasks[0].Cwd != w.Root() || tasks[0].Args[1] != "main.go" || !tasks[0].Default || tasks[0].Env["TEST"] != filepath.Base(w.Root()) {
		t.Fatal(tasks[0])
	}
	if _, err := w.ResolveTask(tasks[1].ID, doc.Path); err == nil {
		t.Fatal("executed dependency task incorrectly")
	}
}

func terminalText(t *testing.T, m *Terminals, id string) string {
	t.Helper()
	snapshot, err := m.Snapshot(id)
	if err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	for _, chunk := range snapshot.Chunks {
		data, err := base64.StdEncoding.DecodeString(chunk.Data)
		if err != nil {
			t.Fatal(err)
		}
		output.Write(data)
	}
	return output.String()
}

func awaitTerminal(t *testing.T, m *Terminals, id string, predicate func(string) bool) string {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		text := terminalText(t, m, id)
		if predicate(text) {
			return text
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("terminal timeout: %s", terminalText(t, m, id))
	return ""
}

func TestRealTerminalInteractiveResizeInterruptAndTask(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash unavailable")
	}
	m := NewTerminals()
	defer m.CloseAll()
	root := t.TempDir()
	info, err := m.Create(TerminalRequest{Shell: "bash", Cwd: root, Columns: 100, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Resize(info.ID, 91, 31); err != nil {
		t.Fatal(err)
	}
	if err := m.Write(info.ID, "test -t 0 && printf 'REAL_%s\\n' PTY; stty size; mkdir child; cd child; pwd\r"); err != nil {
		t.Fatal(err)
	}
	awaitTerminal(t, m, info.ID, func(text string) bool {
		return strings.Contains(text, "REAL_PTY") && strings.Contains(text, "31 91") && strings.Contains(text, filepath.Join(root, "child"))
	})
	if err := m.Write(info.ID, "sleep 30\r"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	if err := m.Write(info.ID, "\x03printf 'INTERRUPT_%s\\n' OK\r"); err != nil {
		t.Fatal(err)
	}
	awaitTerminal(t, m, info.ID, func(text string) bool { return strings.Contains(text, "INTERRUPT_OK") })
	task, err := m.StartTask(Task{Label: "test", Type: "process", Command: "bash", Args: []string{"-c", "printf 'ENV_%s\\n' \"$PRAEWA_TEST\"; exit 7"}, Cwd: root, Env: map[string]string{"PRAEWA_TEST": "VALUE"}})
	if err != nil {
		t.Fatal(err)
	}
	awaitTerminal(t, m, task.ID, func(text string) bool { return strings.Contains(text, "ENV_VALUE") })
	session, _ := m.get(task.ID)
	select {
	case <-session.done:
	case <-time.After(8 * time.Second):
		t.Fatal("task did not exit")
	}
	snapshot, _ := m.Snapshot(task.ID)
	if !snapshot.Info.Exited || snapshot.Info.ExitCode != 7 {
		t.Fatal(snapshot.Info)
	}
	if err := m.Close(info.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Snapshot(info.ID); err == nil {
		t.Fatal("closed terminal is still available")
	}
}

func TestGitStageDiffAndCommit(t *testing.T) {
	w := testWorkspace(t)
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	if err := w.GitInit(); err != nil {
		t.Fatal(err)
	}
	_, _ = w.git("config", "user.name", "Praewa Test")
	_, _ = w.git("config", "user.email", "test@example.invalid")
	doc, _ := w.Create("file with spaces.txt", "first\n")
	if err := w.GitStage(doc.Path, true); err != nil {
		t.Fatal(err)
	}
	if _, err := w.GitCommit("Initial fixture"); err != nil {
		t.Fatal(err)
	}
	if err := w.Save(doc.Path, "second\n"); err != nil {
		t.Fatal(err)
	}
	state := w.GitStatus()
	if !state.Repository || len(state.Files) != 1 || state.Files[0].Worktree != "M" {
		t.Fatal(state)
	}
	diff, err := w.GitDiff(doc.Path, false)
	if err != nil || diff.Original != "first\n" || diff.Modified != "second\n" {
		t.Fatal(diff, err)
	}
	if err := w.GitStage(doc.Path, true); err != nil {
		t.Fatal(err)
	}
	if err := w.GitStage(doc.Path, false); err != nil {
		t.Fatal(err)
	}
	if w.GitStatus().Files[0].Index != " " {
		t.Fatal("unstage failed")
	}
}

func TestSessionRestoresAndWatcherDetects(t *testing.T) {
	t.Setenv("PRAEWA_DATA_DIR", t.TempDir())
	store := NewSessionStore()
	state := Session{Workspace: "/first", OpenFiles: []string{"/first/a.go"}, ActiveFile: "/first/a.go"}
	if err := store.Save(state); err != nil {
		t.Fatal(err)
	}
	loaded := store.Load()
	if loaded.Workspace != state.Workspace || loaded.ActiveFile != state.ActiveFile || len(loaded.Recent) != 1 {
		t.Fatal(loaded)
	}
	root := t.TempDir()
	events := make(chan string, 10)
	watcher := &Watcher{OnChange: func(path string) { events <- path }}
	defer watcher.Close()
	if err := watcher.Start(root); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "watch.txt")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	select {
	case changed := <-events:
		if changed != path {
			t.Fatal(changed)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no filesystem event")
	}
}

func TestGitCommitRejectsHiddenStagedFiles(t *testing.T) {
	w := testWorkspace(t)
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	if err := w.GitInit(); err != nil {
		t.Fatal(err)
	}
	_, _ = w.git("config", "user.name", "Praewa Test")
	_, _ = w.git("config", "user.email", "test@example.invalid")
	document, _ := w.Create("outside.txt", "outside\n")
	_, _ = w.CreateDirectory("child")
	if err := w.GitStage(document.Path, true); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Open(filepath.Join(w.Root(), "child")); err != nil {
		t.Fatal(err)
	}
	if _, err := w.GitCommit("Must not commit hidden files"); err == nil {
		t.Fatal("committed files outside the visible workspace")
	}
}

func TestExternalSourceIsReadableButWorkspaceCannotWriteIt(t *testing.T) {
	w := testWorkspace(t)
	path := filepath.Join(t.TempDir(), "library.go")
	if err := os.WriteFile(path, []byte("package library\n"), 0644); err != nil {
		t.Fatal(err)
	}
	doc, err := ReadText(path)
	if err != nil || doc.Content != "package library\n" {
		t.Fatal(doc, err)
	}
	if _, err := w.SaveChecked(path, "overwrite", doc.Version); err == nil {
		t.Fatal("workspace wrote external source")
	}
}
