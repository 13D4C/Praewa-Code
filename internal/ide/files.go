package ide

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func fileVersion(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (w *Workspace) SaveChecked(path, content, version string) (*Document, error) {
	document, err := w.Read(path)
	if err != nil {
		return nil, err
	}
	if version != "" && document.Version != version {
		return nil, fmt.Errorf("This file changed on disk. Reload it or use Save Anyway to keep your edits")
	}
	if err := w.Save(path, content); err != nil {
		return nil, err
	}
	return w.Read(path)
}

func (w *Workspace) CreateDirectory(path string) (*Entry, error) {
	resolved, err := w.resolve(path, true)
	if err != nil {
		return nil, err
	}
	if err := os.Mkdir(resolved, 0755); err != nil {
		return nil, err
	}
	return &Entry{Name: filepath.Base(resolved), Path: resolved, Directory: true}, nil
}

func (w *Workspace) Rename(path, destination string) (*Entry, error) {
	source, err := w.resolve(path, false)
	if err != nil {
		return nil, err
	}
	if source == w.Root() {
		return nil, fmt.Errorf("The workspace root cannot be renamed")
	}
	target, err := w.resolve(destination, true)
	if err != nil {
		return nil, err
	}
	if source == target {
		return nil, fmt.Errorf("Enter a different name")
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		return nil, fmt.Errorf("The destination already exists")
	}
	if err := os.Rename(source, target); err != nil {
		return nil, err
	}
	info, err := os.Stat(target)
	if err != nil {
		return nil, err
	}
	return &Entry{Name: filepath.Base(target), Path: target, Directory: info.IsDir()}, nil
}

type TrashedFile struct {
	Original string `json:"original"`
	Stored   string `json:"stored"`
}

func (w *Workspace) Trash(path string) (*TrashedFile, error) {
	source, err := w.resolve(path, false)
	if err != nil {
		return nil, err
	}
	if source == w.Root() || inside(filepath.Join(w.Root(), ".git"), source) || source == filepath.Join(w.Root(), ".praewa") || strings.HasPrefix(source, filepath.Join(w.Root(), ".praewa")+string(filepath.Separator)) {
		return nil, fmt.Errorf("This directory cannot be moved to trash")
	}
	trash := filepath.Join(w.Root(), ".praewa", "trash")
	if err := w.makeInternalDirectory(trash); err != nil {
		return nil, err
	}
	holder, err := os.MkdirTemp(trash, "item-")
	if err != nil {
		return nil, err
	}
	stored := filepath.Join(holder, filepath.Base(source))
	if err := os.Rename(source, stored); err != nil {
		return nil, err
	}
	return &TrashedFile{Original: source, Stored: stored}, nil
}

func (w *Workspace) RestoreTrash(item TrashedFile) error {
	stored, err := w.resolve(item.Stored, false)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(stored, filepath.Join(w.Root(), ".praewa", "trash")+string(filepath.Separator)) {
		return fmt.Errorf("Invalid trash entry")
	}
	target, err := w.resolve(item.Original, true)
	if err != nil {
		return err
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		return fmt.Errorf("A file with this name already exists")
	}
	return os.Rename(stored, target)
}

func (w *Workspace) makeInternalDirectory(path string) error {
	root := w.Root()
	if root == "" || !inside(root, path) {
		return fmt.Errorf("Open a workspace first")
	}
	for current := path; current != root; current = filepath.Dir(current) {
		if info, err := os.Lstat(current); err == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("Internal directories cannot be symbolic links")
		}
	}
	return os.MkdirAll(path, 0755)
}

func ignoredDirectory(name string) bool {
	return map[string]bool{".git": true, "node_modules": true, ".praewa": true, "vendor": true, "dist": true, "target": true, "__pycache__": true}[name]
}

func (w *Workspace) FileIndex() ([]Entry, error) {
	root := w.Root()
	if root == "" {
		return []Entry{}, nil
	}
	entries := make([]Entry, 0)
	err := filepath.WalkDir(root, func(path string, item fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if item.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if item.IsDir() {
			if path != root && ignoredDirectory(item.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if len(entries) >= 20000 {
			return fs.SkipAll
		}
		if item.Type().IsRegular() {
			entries = append(entries, Entry{Name: item.Name(), Path: path})
		}
		return nil
	})
	return entries, err
}
