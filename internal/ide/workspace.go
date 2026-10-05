package ide

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"
)

const maxFileSize = 5 * 1024 * 1024

type Entry struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Directory bool   `json:"directory"`
}

type Project struct {
	Name    string  `json:"name"`
	Path    string  `json:"path"`
	Entries []Entry `json:"entries"`
}

type Document struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Content string `json:"content"`
	Version string `json:"version"`
}

type Workspace struct {
	mu   sync.RWMutex
	root string
}

func NewWorkspace() *Workspace {
	return &Workspace{}
}

func (w *Workspace) Root() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.root
}

func (w *Workspace) Open(path string) (*Project, error) {
	root, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("Choose a directory to open a workspace")
	}
	entries, err := list(root)
	if err != nil {
		return nil, err
	}
	w.mu.Lock()
	w.root = root
	w.mu.Unlock()
	return &Project{Name: filepath.Base(root), Path: root, Entries: entries}, nil
}

func (w *Workspace) resolve(path string, creating bool) (string, error) {
	root := w.Root()
	if root == "" {
		return "", fmt.Errorf("Open a workspace first")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	path = filepath.Clean(path)
	if !inside(root, path) {
		return "", fmt.Errorf("The file must be inside the current workspace")
	}
	check := path
	if creating {
		check = filepath.Dir(path)
	}
	real, err := filepath.EvalSymlinks(check)
	if err != nil {
		return "", err
	}
	if !inside(root, real) {
		return "", fmt.Errorf("Links outside the workspace cannot be opened")
	}
	if creating {
		return filepath.Join(real, filepath.Base(path)), nil
	}
	return real, nil
}

func inside(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func list(path string) ([]Entry, error) {
	items, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, 0, len(items))
	ignored := map[string]bool{".git": true, "node_modules": true, ".praewa": true, ".DS_Store": true}
	for _, item := range items {
		if ignored[item.Name()] || item.Type()&os.ModeSymlink != 0 {
			continue
		}
		entries = append(entries, Entry{Name: item.Name(), Path: filepath.Join(path, item.Name()), Directory: item.IsDir()})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Directory != entries[j].Directory {
			return entries[i].Directory
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})
	return entries, nil
}

func (w *Workspace) List(path string) ([]Entry, error) {
	resolved, err := w.resolve(path, false)
	if err != nil {
		return nil, err
	}
	return list(resolved)
}

func (w *Workspace) Read(path string) (*Document, error) {
	resolved, err := w.resolve(path, false)
	if err != nil {
		return nil, err
	}
	return ReadText(resolved)
}

func ReadText(path string) (*Document, error) {
	resolved, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(resolved)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxFileSize {
		return nil, fmt.Errorf("Open a text file smaller than 5 MB")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxFileSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxFileSize || !utf8.Valid(data) || bytes.ContainsRune(data, 0) {
		return nil, fmt.Errorf("This editor supports UTF-8 text files smaller than 5 MB")
	}
	return &Document{Name: filepath.Base(resolved), Path: resolved, Content: string(data), Version: fileVersion(data)}, nil
}

func (w *Workspace) Save(path, content string) error {
	if len(content) > maxFileSize {
		return fmt.Errorf("Files must be smaller than 5 MB")
	}
	resolved, err := w.resolve(path, false)
	if err != nil {
		return err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("Only regular files can be saved")
	}
	file, err := os.CreateTemp(filepath.Dir(resolved), ".praewa-save-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err := file.Chmod(info.Mode().Perm()); err != nil {
		file.Close()
		return err
	}
	if _, err := file.WriteString(content); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), resolved)
}

func (w *Workspace) Create(path, content string) (*Document, error) {
	if len(content) > maxFileSize {
		return nil, fmt.Errorf("Files must be smaller than 5 MB")
	}
	resolved, err := w.resolve(path, true)
	if err != nil {
		return nil, err
	}
	file, err := os.OpenFile(resolved, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		if os.IsExist(err) {
			return nil, fmt.Errorf("A file with this name already exists")
		}
		return nil, err
	}
	_, writeErr := file.WriteString(content)
	closeErr := file.Close()
	if writeErr != nil {
		return nil, writeErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return &Document{Name: filepath.Base(resolved), Path: resolved, Content: content, Version: fileVersion([]byte(content))}, nil
}
