package ide

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/tidwall/jsonc"
)

type Task struct {
	ID      string            `json:"id"`
	Label   string            `json:"label"`
	Type    string            `json:"type"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Cwd     string            `json:"cwd"`
	Env     map[string]string `json:"env"`
	Shell   string            `json:"shell"`
	Source  string            `json:"source"`
	Default bool              `json:"default"`
	Problem string            `json:"problem"`
}

func (w *Workspace) Tasks(file string) ([]Task, error) {
	root := w.Root()
	if root == "" {
		return []Task{}, nil
	}
	tasks := []Task{}
	expand := func(value string) (string, error) {
		variables := map[string]string{"workspaceFolder": root, "workspaceFolderBasename": filepath.Base(root)}
		if file != "" {
			path, err := w.resolve(file, false)
			if err != nil {
				return "", err
			}
			variables["file"] = path
			variables["fileDirname"] = filepath.Dir(path)
			variables["fileBasename"] = filepath.Base(path)
			variables["fileBasenameNoExtension"] = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			variables["relativeFile"], _ = filepath.Rel(root, path)
		}
		var problem error
		value = regexp.MustCompile(`\$\{([^}]+)\}`).ReplaceAllStringFunc(value, func(match string) string {
			key := match[2 : len(match)-1]
			if strings.HasPrefix(key, "env:") {
				return os.Getenv(strings.TrimPrefix(key, "env:"))
			}
			if text, ok := variables[key]; ok {
				return text
			}
			problem = fmt.Errorf("Unsupported or unavailable task variable: %s", match)
			return match
		})
		return value, problem
	}
	if data, err := os.ReadFile(filepath.Join(root, ".vscode", "tasks.json")); err == nil {
		var config struct {
			Tasks []map[string]json.RawMessage `json:"tasks"`
		}
		if err := json.Unmarshal(jsonc.ToJSON(data), &config); err != nil {
			return nil, fmt.Errorf("Invalid .vscode/tasks.json: %w", err)
		}
		for i, fields := range config.Tasks {
			platform := map[string]string{"windows": "windows", "linux": "linux", "darwin": "osx"}[runtime.GOOS]
			var override map[string]json.RawMessage
			_ = json.Unmarshal(fields[platform], &override)
			for key, value := range override {
				fields[key] = value
			}
			bytes, _ := json.Marshal(fields)
			var task Task
			if err := json.Unmarshal(bytes, &task); err != nil {
				return nil, fmt.Errorf("Invalid task %d: %w", i+1, err)
			}
			task.ID = fmt.Sprintf("vscode:%d", i)
			task.Source = ".vscode/tasks.json"
			if task.Label == "" {
				task.Label = task.Command
			}
			if task.Type == "" {
				task.Type = "shell"
			}
			var options struct {
				Cwd   string            `json:"cwd"`
				Env   map[string]string `json:"env"`
				Shell struct {
					Executable string   `json:"executable"`
					Args       []string `json:"args"`
				} `json:"shell"`
			}
			_ = json.Unmarshal(fields["options"], &options)
			task.Cwd = options.Cwd
			task.Env = options.Env
			task.Shell = options.Shell.Executable
			if task.Cwd == "" {
				task.Cwd = root
			}
			if task.Type != "shell" && task.Type != "process" {
				task.Problem = "This task requires a VS Code extension"
			}
			if len(fields["dependsOn"]) > 0 {
				task.Problem = "Task dependencies are not supported yet; run each task separately"
			}
			if len(options.Shell.Args) > 0 {
				task.Problem = "Custom shell arguments are not supported yet"
			}
			for _, key := range []string{"isBackground", "runOptions"} {
				if string(fields[key]) == "true" {
					task.Problem = "Background task scheduling is not supported yet"
				}
			}
			var group struct {
				Kind      string `json:"kind"`
				IsDefault bool   `json:"isDefault"`
			}
			if json.Unmarshal(fields["group"], &group) == nil {
				task.Default = group.Kind == "build" && group.IsDefault
			}
			values := []*string{&task.Command, &task.Cwd, &task.Shell}
			for j := range task.Args {
				values = append(values, &task.Args[j])
			}
			for _, value := range values {
				next, err := expand(*value)
				*value = next
				if err != nil {
					task.Problem = err.Error()
				}
			}
			for key, value := range task.Env {
				next, err := expand(value)
				task.Env[key] = next
				if err != nil {
					task.Problem = err.Error()
				}
			}
			if !filepath.IsAbs(task.Cwd) {
				task.Cwd = filepath.Join(root, task.Cwd)
			}
			tasks = append(tasks, task)
		}
	}
	entries, err := w.FileIndex()
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		dir := filepath.Dir(entry.Path)
		relative, _ := filepath.Rel(root, dir)
		prefix := ""
		if relative != "." {
			prefix = filepath.ToSlash(relative) + ": "
		}
		add := func(label, command string, args []string, defaultBuild bool) {
			tasks = append(tasks, Task{ID: entry.Path + ":" + label, Label: prefix + label, Type: "process", Command: command, Args: args, Cwd: dir, Source: entry.Name, Default: defaultBuild && dir == root})
		}
		switch entry.Name {
		case "package.json":
			data, err := os.ReadFile(entry.Path)
			if err != nil || len(data) > maxFileSize {
				continue
			}
			var pkg struct {
				Scripts map[string]string `json:"scripts"`
			}
			if json.Unmarshal(data, &pkg) != nil {
				continue
			}
			names := []string{}
			for name := range pkg.Scripts {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, name := range names {
				add("npm: "+name, "npm", []string{"run", name}, name == "build")
			}
		case "go.mod":
			add("Go: build", "go", []string{"build", "./..."}, true)
			add("Go: run", "go", []string{"run", "."}, false)
			add("Go: test", "go", []string{"test", "./..."}, false)
		case "Cargo.toml":
			add("Rust: build", "cargo", []string{"build"}, true)
			add("Rust: run", "cargo", []string{"run"}, false)
			add("Rust: test", "cargo", []string{"test"}, false)
		case "Makefile":
			data, _ := os.ReadFile(entry.Path)
			seen := map[string]bool{}
			for _, match := range regexp.MustCompile(`(?m)^([A-Za-z][A-Za-z0-9_-]*):(?:\s|$)`).FindAllStringSubmatch(string(data), 100) {
				if !seen[match[1]] {
					add("make: "+match[1], "make", []string{match[1]}, match[1] == "build")
					seen[match[1]] = true
				}
			}
		}
	}
	if file != "" {
		path, err := w.resolve(file, false)
		if err != nil {
			return nil, err
		}
		command := ""
		args := []string{path}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".py":
			command = "python3"
			if runtime.GOOS == "windows" {
				command = "python"
			}
		case ".js", ".mjs", ".cjs":
			command = "node"
		case ".go":
			command = "go"
			args = []string{"run", path}
		case ".sh":
			command = "bash"
		}
		if command != "" {
			tasks = append(tasks, Task{ID: "file:run", Label: "Run active file", Type: "process", Command: command, Args: args, Cwd: filepath.Dir(path), Source: "Active file"})
		}
	}
	return tasks, nil
}

func (w *Workspace) ResolveTask(id, file string) (*Task, error) {
	tasks, err := w.Tasks(file)
	if err != nil {
		return nil, err
	}
	for _, task := range tasks {
		if task.ID == id {
			if task.Problem != "" {
				return nil, fmt.Errorf("%s", task.Problem)
			}
			return &task, nil
		}
	}
	return nil, fmt.Errorf("This task is no longer available")
}
