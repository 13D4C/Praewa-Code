package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"praewa-code/internal/ide"
)

func TestInitialStatePreservesOpenFiles(t *testing.T) {
	t.Setenv("PRAEWA_DATA_DIR", t.TempDir())
	root := t.TempDir()
	path := filepath.Join(root, "main.go")
	if err := os.WriteFile(path, []byte("package main\nfunc main() {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	app := NewApp()
	defer app.shutdown(context.Background())
	if err := app.sessions.Save(ide.Session{Workspace: root, OpenFiles: []string{path}, ActiveFile: path}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		state, err := app.InitialState()
		if err != nil || state.Project == nil {
			t.Fatal(state, err)
		}
		if len(state.Session.OpenFiles) != 1 || state.Session.ActiveFile != path {
			t.Fatal("initial state lost open files", state.Session)
		}
		if len(app.sessions.Load().OpenFiles) != 1 {
			t.Fatal("loading the session changed its persisted files")
		}
	}
}
