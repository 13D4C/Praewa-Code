package ide

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type Session struct {
	Recent     []string `json:"recent"`
	Workspace  string   `json:"workspace"`
	OpenFiles  []string `json:"openFiles"`
	ActiveFile string   `json:"activeFile"`
}

type SessionStore struct {
	mu   sync.Mutex
	path string
}

func NewSessionStore() *SessionStore {
	directory := os.Getenv("PRAEWA_DATA_DIR")
	if directory == "" {
		if config, err := os.UserConfigDir(); err == nil {
			directory = filepath.Join(config, "praewa-code")
		}
	}
	return &SessionStore{path: filepath.Join(directory, "session.json")}
}

func (s *SessionStore) Load() Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load()
}

func (s *SessionStore) load() Session {
	state := Session{Recent: []string{}, OpenFiles: []string{}}
	data, err := os.ReadFile(s.path)
	if err == nil {
		_ = json.Unmarshal(data, &state)
	}
	return state
}

func (s *SessionStore) Save(state Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	previous := s.load()
	state.Recent = previous.Recent
	if state.Workspace != "" {
		state.Recent = []string{state.Workspace}
		for _, path := range previous.Recent {
			if path != state.Workspace && len(state.Recent) < 10 {
				state.Recent = append(state.Recent, path)
			}
		}
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(s.path), "session-")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), s.path)
}
