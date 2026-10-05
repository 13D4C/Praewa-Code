package ide

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestInstalledGoLanguageServer(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls is not installed")
	}
	w := testWorkspace(t)
	_, _ = w.Create("go.mod", "module languagefixture\n\ngo 1.23.0\n")
	doc, _ := w.Create("main.go", "package main\nimport \"fmt\"\nfunc main() { fmt.Println(unknownSymbol) }\n")
	languages := NewLanguages(w)
	defer languages.Close()
	events := make(chan LanguageDiagnostics, 64)
	languages.OnDiagnostics = func(event LanguageDiagnostics) {
		select {
		case events <- event:
		default:
		}
	}
	status := languages.Sync(doc.Path, doc.Content)
	if !status.Available {
		t.Fatal(status)
	}
	deadline := time.After(20 * time.Second)
	found := false
	for !found {
		select {
		case event := <-events:
			for _, item := range event.Diagnostics {
				if strings.Contains(string(item), "unknownSymbol") {
					found = true
				}
			}
		case <-deadline:
			t.Fatal("missing Go diagnostic")
		}
	}
	text := "package main\nimport \"fmt\"\nfunc main() { fmt.Pr }\n"
	result, err := languages.Request(doc.Path, text, "completion", 2, 20)
	if err != nil || !strings.Contains(string(result), "Println") {
		t.Fatal(string(result), err)
	}
	text = "package main\nfunc greeting() string {return \"hello\"}\nfunc main() {greeting()}\n"
	result, err = languages.Request(doc.Path, text, "definition", 2, 15)
	if err != nil || !strings.Contains(string(result), "main.go") {
		t.Fatal(string(result), err)
	}
	result, err = languages.Request(doc.Path, text, "formatting", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	var edits []map[string]any
	if json.Unmarshal(result, &edits) != nil || len(edits) == 0 {
		t.Fatal("missing formatting edits", string(result))
	}
	languages.CloseDocument(doc.Path)
}

func TestLanguageURIRoundTrip(t *testing.T) {
	path := "/tmp/project with spaces/ไทย.go"
	if uriFile(fileURI(path)) != path {
		t.Fatal(fileURI(path))
	}
}
