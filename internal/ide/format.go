package ide

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func (w *Workspace) Format(path, content string) (string, error) {
	resolved, err := w.resolve(path, false)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var cmd *exec.Cmd
	if filepath.Ext(path) == ".go" {
		cmd = exec.CommandContext(ctx, "gofmt")
	} else if filepath.Ext(path) == ".json" {
		var output bytes.Buffer
		if err := json.Indent(&output, []byte(content), "", "  "); err != nil {
			return "", err
		}
		return output.String() + "\n", nil
	} else {
		for dir := filepath.Dir(resolved); inside(w.Root(), dir); dir = filepath.Dir(dir) {
			candidate := filepath.Join(dir, "node_modules", "prettier", "bin", "prettier.cjs")
			if _, err := os.Stat(candidate); err == nil {
				cmd = exec.CommandContext(ctx, "node", candidate, "--stdin-filepath", resolved)
				break
			}
			if dir == w.Root() {
				break
			}
		}
		if cmd == nil {
			return "", fmt.Errorf("Install Prettier in this project to format this language")
		}
	}
	cmd.Dir = filepath.Dir(resolved)
	cmd.Stdin = strings.NewReader(content)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	data, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s: %s", err, stderr.String())
	}
	return string(data), nil
}
