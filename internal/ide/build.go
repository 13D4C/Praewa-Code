package ide

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type BuildResult struct {
	Command   string `json:"command"`
	Output    string `json:"output"`
	ExitCode  int    `json:"exitCode"`
	Duration  int64  `json:"duration"`
	Artifact  string `json:"artifact"`
	Cancelled bool   `json:"cancelled"`
}

type plan struct {
	command  string
	args     []string
	dir      string
	artifact string
}

type Builder struct {
	workspace *Workspace
	mu        sync.Mutex
	cancel    context.CancelFunc
	OnOutput  func(string)
}

func NewBuilder(workspace *Workspace) *Builder {
	return &Builder{workspace: workspace}
}

func (b *Builder) Running() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.cancel != nil
}

func (b *Builder) Cancel() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cancel != nil {
		b.cancel()
	}
}

func (b *Builder) Build(parent context.Context, path string) (*BuildResult, error) {
	b.mu.Lock()
	if b.cancel != nil {
		b.mu.Unlock()
		return nil, fmt.Errorf("A build is already running")
	}
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	b.cancel = cancel
	b.mu.Unlock()
	defer func() {
		cancel()
		b.mu.Lock()
		b.cancel = nil
		b.mu.Unlock()
	}()
	resolved, err := b.workspace.resolve(path, false)
	if err != nil {
		return nil, err
	}
	if _, err := b.workspace.Read(resolved); err != nil {
		return nil, err
	}
	p, err := b.plan(resolved)
	if err != nil {
		return nil, err
	}
	if _, err := exec.LookPath(p.command); err != nil {
		return nil, fmt.Errorf("%s is not installed or is missing from PATH", p.command)
	}
	command := strings.Join(append([]string{p.command}, p.args...), " ")
	writer := &outputWriter{emit: b.OnOutput}
	cmd := exec.CommandContext(ctx, p.command, p.args...)
	cmd.Dir = p.dir
	cmd.Stdout = writer
	cmd.Stderr = writer
	configureProcess(cmd)
	if b.OnOutput != nil {
		b.OnOutput("$ " + command + "\n\n")
	}
	start := time.Now()
	err = cmd.Run()
	result := &BuildResult{Command: command, Output: writer.String(), Duration: time.Since(start).Milliseconds(), Artifact: p.artifact}
	if err != nil {
		result.ExitCode = -1
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			result.ExitCode = exit.ExitCode()
		} else if ctx.Err() == nil {
			return nil, fmt.Errorf("Could not start build: %w", err)
		}
		result.Artifact = ""
	}
	if ctx.Err() != nil {
		result.Cancelled = true
		result.Artifact = ""
		result.ExitCode = -1
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			result.Output += "\nBuild exceeded the 2-minute time limit.\n"
			if b.OnOutput != nil {
				b.OnOutput("\nBuild exceeded the 2-minute time limit.\n")
			}
		}
	}
	return result, nil
}

func (b *Builder) plan(path string) (*plan, error) {
	dir := filepath.Dir(path)
	root := b.workspace.Root()
	ext := strings.ToLower(filepath.Ext(path))
	name := strings.TrimSuffix(filepath.Base(path), ext)
	output := filepath.Join(root, ".praewa", "build")
	artifact := filepath.Join(output, name)
	if runtime.GOOS == "windows" {
		artifact += ".exe"
	}
	p := &plan{dir: dir}
	if ext == ".go" || ext == ".rs" || ext == ".c" || ext == ".cpp" || ext == ".cc" || ext == ".cxx" {
		if existing, err := filepath.EvalSymlinks(output); err == nil && !inside(root, existing) {
			return nil, fmt.Errorf("Build output must stay inside the workspace")
		}
		if info, err := os.Lstat(filepath.Join(root, ".praewa")); err == nil && info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("Build output directory cannot be a symbolic link")
		}
		if err := os.MkdirAll(output, 0755); err != nil {
			return nil, err
		}
		if info, err := os.Lstat(artifact); err == nil && !info.Mode().IsRegular() {
			return nil, fmt.Errorf("Build artifact cannot be a symbolic link or directory")
		}
	}
	switch ext {
	case ".go":
		p.command, p.artifact = "go", artifact
		if module := findManifest(dir, root, "go.mod"); module != "" {
			p.args = []string{"build", "-buildvcs=false", "-o", artifact, "."}
		} else {
			p.args = []string{"build", "-buildvcs=false", "-o", artifact, filepath.Base(path)}
		}
	case ".js", ".mjs", ".cjs", ".ts", ".tsx", ".jsx", ".svelte":
		manifest := findManifest(dir, root, "package.json")
		if manifest != "" {
			data, err := os.ReadFile(filepath.Join(manifest, "package.json"))
			if err != nil {
				return nil, err
			}
			var pkg struct {
				Scripts map[string]string `json:"scripts"`
			}
			if err := json.Unmarshal(data, &pkg); err != nil {
				return nil, fmt.Errorf("Invalid package.json: %w", err)
			}
			if pkg.Scripts["build"] != "" {
				p.command, p.args, p.dir = "npm", []string{"run", "build"}, manifest
				break
			}
		}
		if ext == ".js" || ext == ".mjs" || ext == ".cjs" {
			p.command, p.args = "node", []string{"--check", filepath.Base(path)}
		} else {
			return nil, fmt.Errorf("Add a build script to package.json and install the project dependencies first")
		}
	case ".py":
		p.command = "python3"
		if runtime.GOOS == "windows" {
			p.command = "python"
		}
		p.args = []string{"-m", "py_compile", filepath.Base(path)}
	case ".rs":
		if manifest := findManifest(dir, root, "Cargo.toml"); manifest != "" {
			p.command, p.args, p.dir = "cargo", []string{"build"}, manifest
		} else {
			p.command, p.args, p.artifact = "rustc", []string{filepath.Base(path), "-o", artifact}, artifact
		}
	case ".c":
		p.command, p.args, p.artifact = "gcc", []string{"-Wall", "-Wextra", filepath.Base(path), "-o", artifact}, artifact
	case ".cpp", ".cc", ".cxx":
		p.command, p.args, p.artifact = "g++", []string{"-Wall", "-Wextra", "-std=c++17", filepath.Base(path), "-o", artifact}, artifact
	default:
		return nil, fmt.Errorf("Choose a Go, JavaScript, TypeScript, Svelte, Python, Rust, C, or C++ file to compile")
	}
	return p, nil
}

func findManifest(dir, root, filename string) string {
	for inside(root, dir) {
		if info, err := os.Stat(filepath.Join(dir, filename)); err == nil && info.Mode().IsRegular() {
			return dir
		}
		if dir == root {
			break
		}
		dir = filepath.Dir(dir)
	}
	return ""
}

type outputWriter struct {
	mu        sync.Mutex
	text      strings.Builder
	emit      func(string)
	truncated bool
}

func (w *outputWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	remaining := 1024*1024 - w.text.Len()
	if remaining > 0 {
		chunk := p
		if len(chunk) > remaining {
			chunk = chunk[:remaining]
		}
		w.text.Write(chunk)
		if w.emit != nil {
			w.emit(string(chunk))
		}
	}
	if len(p) > remaining && !w.truncated {
		w.truncated = true
		message := "\nOutput truncated at 1 MB.\n"
		w.text.WriteString(message)
		if w.emit != nil {
			w.emit(message)
		}
	}
	return len(p), nil
}

func (w *outputWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.text.String()
}
