package ide

import (
	"os"
	"path/filepath"
	"testing"
)

func openTestWorkspace(t *testing.T) (*Workspace, string) {
	t.Helper()
	root := t.TempDir()
	w := NewWorkspace()
	if _, err := w.Open(root); err != nil {
		t.Fatal(err)
	}
	return w, root
}

func TestWorkspaceFileLifecycle(t *testing.T) {
	w, root := openTestWorkspace(t)
	document, err := w.Create("main.go", "package main\n")
	if err != nil {
		t.Fatal(err)
	}
	if document.Path != filepath.Join(root, "main.go") {
		t.Fatalf("unexpected path %s", document.Path)
	}
	if err := w.Save(document.Path, "package main\n\nfunc main() {}\n"); err != nil {
		t.Fatal(err)
	}
	loaded, err := w.Read(document.Path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Content != "package main\n\nfunc main() {}\n" {
		t.Fatalf("saved content was not preserved: %q", loaded.Content)
	}
	if _, err := w.Create("main.go", "overwrite"); err == nil {
		t.Fatal("existing file was overwritten")
	}
	loaded, err = w.Read(document.Path)
	if err != nil || loaded.Content == "overwrite" {
		t.Fatal("duplicate creation changed existing file")
	}
}

func TestWorkspaceRejectsEscapingPaths(t *testing.T) {
	w, root := openTestWorkspace(t)
	outside := t.TempDir()
	external := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(external, []byte("outside"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{external, "../secret.txt"} {
		if _, err := w.Read(path); err == nil {
			t.Fatalf("read escaped root through %s", path)
		}
		if err := w.Save(path, "changed"); err == nil {
			t.Fatalf("save escaped root through %s", path)
		}
		if _, err := w.Create(path, "changed"); err == nil {
			t.Fatalf("create escaped root through %s", path)
		}
	}
	link := filepath.Join(root, "linked")
	if err := os.Symlink(outside, link); err != nil {
		t.Skip(err)
	}
	if _, err := w.Read(filepath.Join(link, "secret.txt")); err == nil {
		t.Fatal("read followed an external link")
	}
	if _, err := w.Create(filepath.Join(link, "new.txt"), "changed"); err == nil {
		t.Fatal("create followed an external link")
	}
	data, err := os.ReadFile(external)
	if err != nil || string(data) != "outside" {
		t.Fatal("external file was modified")
	}
}

func TestWorkspaceRejectsBinaryAndLargeFiles(t *testing.T) {
	w, root := openTestWorkspace(t)
	for _, item := range []struct {
		name string
		data []byte
	}{
		{"binary.dat", []byte{0, 1, 2}},
		{"invalid.txt", []byte{0xff, 0xfe}},
		{"large.txt", make([]byte, maxFileSize+1)},
	} {
		if err := os.WriteFile(filepath.Join(root, item.name), item.data, 0644); err != nil {
			t.Fatal(err)
		}
		if _, err := w.Read(item.name); err == nil {
			t.Fatalf("accepted %s", item.name)
		}
	}
}

func TestWorkspaceListsDirectoriesFirst(t *testing.T) {
	w, root := openTestWorkspace(t)
	for _, name := range []string{"src", ".git", "node_modules", ".praewa"} {
		if err := os.Mkdir(filepath.Join(root, name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := w.Create("a.go", "package main"); err != nil {
		t.Fatal(err)
	}
	entries, err := w.List(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Name != "src" || entries[1].Name != "a.go" {
		t.Fatalf("unexpected entries: %+v", entries)
	}
}
