package ide

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestBuildGoAndCompilerFailure(t *testing.T) {
	w, root := openTestWorkspace(t)
	if _, err := w.Create("go.mod", "module test-project\n\ngo 1.23.0\n"); err != nil {
		t.Fatal(err)
	}
	document, err := w.Create("main.go", "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"hello\") }\n")
	if err != nil {
		t.Fatal(err)
	}
	builder := NewBuilder(w)
	var streamed strings.Builder
	builder.OnOutput = func(text string) { streamed.WriteString(text) }
	result, err := builder.Build(context.Background(), document.Path)
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 || result.Cancelled || result.Artifact == "" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if !strings.HasPrefix(result.Artifact, filepath.Join(root, ".praewa", "build")) {
		t.Fatalf("artifact escaped build directory: %s", result.Artifact)
	}
	output, err := exec.Command(result.Artifact).Output()
	if err != nil || string(output) != "hello\n" {
		t.Fatalf("compiled program failed: %q, %v", output, err)
	}
	if err := w.Save(document.Path, "package main\nfunc main() { broken syntax }\n"); err != nil {
		t.Fatal(err)
	}
	streamed.Reset()
	result, err = builder.Build(context.Background(), document.Path)
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode == 0 || result.Artifact != "" || result.Output == "" {
		t.Fatalf("compiler error was not reported: %+v", result)
	}
	if !strings.Contains(streamed.String(), "syntax error") {
		t.Fatalf("compiler output was not streamed: %s", streamed.String())
	}
}

func TestBuildStopsAndRejectsConcurrentBuild(t *testing.T) {
	if _, err := exec.LookPath("npm"); err != nil {
		t.Skip("npm unavailable")
	}
	w, _ := openTestWorkspace(t)
	if _, err := w.Create("package.json", `{"scripts":{"build":"node -e \"console.log('STARTED');setInterval(() => {}, 1000)\""}}`); err != nil {
		t.Fatal(err)
	}
	document, err := w.Create("main.js", "console.log('hello')\n")
	if err != nil {
		t.Fatal(err)
	}
	builder := NewBuilder(w)
	ready := make(chan struct{})
	var once sync.Once
	builder.OnOutput = func(text string) {
		if strings.Contains(text, "STARTED") {
			once.Do(func() { close(ready) })
		}
	}
	type completion struct {
		result *BuildResult
		err    error
	}
	done := make(chan completion, 1)
	go func() {
		result, err := builder.Build(context.Background(), document.Path)
		done <- completion{result, err}
	}()
	defer builder.Cancel()
	select {
	case <-ready:
	case completed := <-done:
		t.Fatalf("build stopped before starting: %+v, %v", completed.result, completed.err)
	case <-time.After(15 * time.Second):
		t.Fatal("build did not start")
	}
	if _, err := builder.Build(context.Background(), document.Path); err == nil {
		t.Fatal("concurrent build was accepted")
	}
	builder.Cancel()
	select {
	case completed := <-done:
		if completed.err != nil || !completed.result.Cancelled || completed.result.ExitCode == 0 {
			t.Fatalf("cancellation failed: %+v", completed)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("build did not stop")
	}
	if builder.Running() {
		t.Fatal("builder remained busy after cancellation")
	}
}

func TestBuildRejectsOutputDirectoryLink(t *testing.T) {
	w, root := openTestWorkspace(t)
	if _, err := w.Create("main.go", "package main\nfunc main() {}\n"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(root, ".praewa")); err != nil {
		t.Skip(err)
	}
	if _, err := NewBuilder(w).Build(context.Background(), "main.go"); err == nil {
		t.Fatal("build followed output directory link")
	}
}

func TestBuildRejectsArtifactLink(t *testing.T) {
	w, root := openTestWorkspace(t)
	if _, err := w.Create("main.go", "package main\nfunc main() {}\n"); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, ".praewa", "build")
	if err := os.MkdirAll(output, 0755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "existing")
	if err := os.WriteFile(outside, []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(output, "main")
	if err := os.Symlink(outside, artifact); err != nil {
		t.Skip(err)
	}
	if _, err := NewBuilder(w).Build(context.Background(), "main.go"); err == nil {
		t.Fatal("build followed artifact link")
	}
	data, err := os.ReadFile(outside)
	if err != nil || string(data) != "keep" {
		t.Fatal("external artifact was changed")
	}
}
