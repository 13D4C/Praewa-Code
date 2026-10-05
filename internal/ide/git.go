package ide

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type GitFile struct {
	Path     string `json:"path"`
	Original string `json:"original"`
	Index    string `json:"index"`
	Worktree string `json:"worktree"`
}
type GitState struct {
	Available  bool      `json:"available"`
	Repository bool      `json:"repository"`
	Branch     string    `json:"branch"`
	Files      []GitFile `json:"files"`
	Log        string    `json:"log"`
	Problem    string    `json:"problem"`
}
type Diff struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Original string `json:"original"`
	Modified string `json:"modified"`
	Patch    string `json:"patch"`
}

func (w *Workspace) git(args ...string) (string, error) {
	if w.Root() == "" {
		return "", fmt.Errorf("Open a workspace first")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", w.Root()}, args...)...)
	data, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s", strings.TrimSpace(string(data)))
	}
	return string(data), nil
}

func (w *Workspace) GitStatus() *GitState {
	state := &GitState{Files: []GitFile{}}
	if _, err := exec.LookPath("git"); err != nil {
		state.Problem = "Git is not installed or is missing from PATH"
		return state
	}
	state.Available = true
	if _, err := w.git("rev-parse", "--show-toplevel"); err != nil {
		return state
	}
	state.Repository = true
	branch, _ := w.git("branch", "--show-current")
	state.Branch = strings.TrimSpace(branch)
	if state.Branch == "" {
		branch, _ = w.git("rev-parse", "--short", "HEAD")
		state.Branch = strings.TrimSpace(branch)
	}
	data, err := w.git("status", "--porcelain=v1", "-z", "--untracked-files=all", "--", w.Root())
	if err != nil {
		state.Problem = err.Error()
		return state
	}
	parts := strings.Split(data, "\x00")
	top, _ := w.git("rev-parse", "--show-toplevel")
	top = strings.TrimSpace(top)
	for i := 0; i < len(parts); i++ {
		part := parts[i]
		if len(part) < 4 {
			continue
		}
		file := GitFile{Path: filepath.Join(top, part[3:]), Index: part[:1], Worktree: part[1:2]}
		if strings.ContainsAny(part[:2], "RC") && i+1 < len(parts) {
			i++
			file.Original = filepath.Join(top, parts[i])
		}
		if inside(w.Root(), file.Path) && !strings.HasPrefix(file.Path, filepath.Join(w.Root(), ".praewa")+string(filepath.Separator)) {
			state.Files = append(state.Files, file)
		}
	}
	state.Log, _ = w.git("log", "-5", "--format=%h %s")
	return state
}

func (w *Workspace) gitPath(path string) (string, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(w.Root(), path)
	}
	if !inside(w.Root(), filepath.Clean(path)) {
		return "", fmt.Errorf("Choose a file inside the workspace")
	}
	return path, nil
}

func (w *Workspace) GitStage(path string, staged bool) error {
	path, err := w.gitPath(path)
	if err != nil {
		return err
	}
	if staged {
		_, err = w.git("add", "--", path)
	} else {
		if _, headErr := w.git("rev-parse", "--verify", "HEAD"); headErr == nil {
			_, err = w.git("restore", "--staged", "--", path)
		} else {
			_, err = w.git("rm", "--cached", "--", path)
		}
	}
	return err
}

func (w *Workspace) GitDiff(path string, staged bool) (*Diff, error) {
	path, err := w.gitPath(path)
	if err != nil {
		return nil, err
	}
	top, err := w.git("rev-parse", "--show-toplevel")
	if err != nil {
		return nil, err
	}
	relative, _ := filepath.Rel(strings.TrimSpace(top), path)
	relative = filepath.ToSlash(relative)
	diff := &Diff{Name: filepath.Base(path), Path: path}
	oldRelative := relative
	for _, file := range w.GitStatus().Files {
		if file.Path == path && file.Original != "" {
			oldRelative, _ = filepath.Rel(strings.TrimSpace(top), file.Original)
			oldRelative = filepath.ToSlash(oldRelative)
			break
		}
	}
	if staged {
		diff.Original, _ = w.git("show", "HEAD:"+oldRelative)
		diff.Modified, _ = w.git("show", ":"+relative)
		diff.Patch, _ = w.git("diff", "--cached", "--", path)
	} else {
		diff.Original, _ = w.git("show", ":"+relative)
		if diff.Original == "" && oldRelative != relative {
			diff.Original, _ = w.git("show", ":"+oldRelative)
		}
		if doc, err := w.Read(path); err == nil {
			diff.Modified = doc.Content
		}
		diff.Patch, _ = w.git("diff", "--", path)
	}
	if len(diff.Original) > maxFileSize || len(diff.Modified) > maxFileSize || strings.ContainsRune(diff.Original+diff.Modified, 0) {
		return nil, fmt.Errorf("This diff is too large or contains binary data")
	}
	return diff, nil
}

func (w *Workspace) GitCommit(message string) (string, error) {
	if strings.TrimSpace(message) == "" {
		return "", fmt.Errorf("Enter a commit message")
	}
	top, err := w.git("rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	staged, err := w.git("-c", "diff.relative=false", "diff", "--cached", "--name-only", "-z")
	if err != nil {
		return "", err
	}
	for _, name := range strings.Split(staged, "\x00") {
		if name == "" {
			continue
		}
		path := filepath.Join(strings.TrimSpace(top), name)
		if !inside(w.Root(), path) || inside(filepath.Join(w.Root(), ".praewa"), path) {
			return "", fmt.Errorf("There are staged files outside the visible workspace. Open the repository root or review them in Terminal before committing")
		}
	}
	return w.git("commit", "-m", message)
}

func (w *Workspace) GitInit() error { _, err := w.git("init"); return err }
