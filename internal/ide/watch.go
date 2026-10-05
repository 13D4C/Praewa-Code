package ide

import (
	"io/fs"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

type Watcher struct {
	mu       sync.Mutex
	watcher  *fsnotify.Watcher
	OnChange func(string)
}

func (w *Watcher) Start(root string) error {
	w.Close()
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	w.mu.Lock()
	w.watcher = watcher
	w.mu.Unlock()
	add := func(path string) {
		_ = filepath.WalkDir(path, func(name string, entry fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if entry.IsDir() {
				if name != root && ignoredDirectory(entry.Name()) {
					return filepath.SkipDir
				}
				_ = watcher.Add(name)
			}
			return nil
		})
	}
	add(root)
	go func() {
		var timer *time.Timer
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					if timer != nil {
						timer.Stop()
					}
					return
				}
				if event.Has(fsnotify.Create) {
					add(event.Name)
				}
				if timer != nil {
					timer.Stop()
				}
				path := event.Name
				timer = time.AfterFunc(180*time.Millisecond, func() {
					if w.OnChange != nil {
						w.OnChange(path)
					}
				})
			case _, ok := <-watcher.Errors:
				if !ok {
					return
				}
			}
		}
	}()
	return nil
}

func (w *Watcher) Close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.watcher != nil {
		_ = w.watcher.Close()
		w.watcher = nil
	}
}
