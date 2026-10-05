package ide

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aymanbagabas/go-pty"
)

type ShellProfile struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Default bool   `json:"default"`
}

type TerminalInfo struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Cwd      string `json:"cwd"`
	Shell    string `json:"shell"`
	PID      int    `json:"pid"`
	Exited   bool   `json:"exited"`
	ExitCode int    `json:"exitCode"`
	Task     bool   `json:"task"`
}

type TerminalEvent struct {
	ID       string `json:"id"`
	Sequence uint64 `json:"sequence"`
	Data     string `json:"data"`
	Exited   bool   `json:"exited"`
	ExitCode int    `json:"exitCode"`
}

type TerminalSnapshot struct {
	Info     TerminalInfo    `json:"info"`
	Chunks   []TerminalEvent `json:"chunks"`
	Sequence uint64          `json:"sequence"`
}

type TerminalRequest struct {
	Shell   string `json:"shell"`
	Cwd     string `json:"cwd"`
	Columns int    `json:"columns"`
	Rows    int    `json:"rows"`
}

type terminalSession struct {
	mu        sync.Mutex
	writeMu   sync.Mutex
	info      TerminalInfo
	pty       pty.Pty
	cmd       *pty.Cmd
	chunks    []TerminalEvent
	bytes     int
	sequence  uint64
	done      chan struct{}
	once      sync.Once
	closeOnce sync.Once
}

type Terminals struct {
	mu           sync.Mutex
	sessions     map[string]*terminalSession
	next         atomic.Uint64
	OnEvent      func(TerminalEvent)
	OnTaskOutput func(string, string)
}

func NewTerminals() *Terminals { return &Terminals{sessions: map[string]*terminalSession{}} }

func ShellProfiles() []ShellProfile {
	defaultShell := os.Getenv("SHELL")
	names := []string{defaultShell, "bash", "zsh", "fish", "sh"}
	if runtime.GOOS == "windows" {
		names = []string{"pwsh", "powershell", "cmd"}
		defaultShell = ""
	}
	profiles := []ShellProfile{}
	seen := map[string]bool{}
	for _, name := range names {
		if name == "" {
			continue
		}
		path, err := exec.LookPath(name)
		if err != nil || seen[path] {
			continue
		}
		seen[path] = true
		profiles = append(profiles, ShellProfile{Name: filepath.Base(path), Path: path, Default: len(profiles) == 0})
	}
	return profiles
}

func DefaultShell() string {
	profiles := ShellProfiles()
	if len(profiles) > 0 {
		return profiles[0].Path
	}
	return ""
}

func shellCommandArgs(shell, command string) []string {
	name := strings.ToLower(filepath.Base(shell))
	if runtime.GOOS == "windows" {
		if name == "cmd.exe" || name == "cmd" {
			return []string{"/D", "/S", "/C", command}
		}
		return []string{"-NoLogo", "-NoProfile", "-Command", command}
	}
	return []string{"-c", command}
}

func (m *Terminals) Create(request TerminalRequest) (*TerminalInfo, error) {
	shell := request.Shell
	if shell == "" {
		shell = DefaultShell()
	}
	if shell == "" {
		return nil, fmt.Errorf("No shell was found on this computer")
	}
	if request.Cwd == "" {
		request.Cwd, _ = os.UserHomeDir()
	}
	args := []string{}
	if runtime.GOOS != "windows" {
		args = []string{"-i"}
	}
	return m.start(filepath.Base(shell), request.Cwd, shell, args, nil, false, request.Columns, request.Rows)
}

func (m *Terminals) StartTask(task Task) (*TerminalInfo, error) {
	command, args := task.Command, task.Args
	if task.Type != "process" {
		command = task.Shell
		if command == "" {
			command = DefaultShell()
		}
		line := task.Command
		for _, arg := range task.Args {
			line += " " + ShellQuote(arg, command)
		}
		args = shellCommandArgs(command, line)
	}
	return m.start(task.Label, task.Cwd, command, args, task.Env, true, 100, 24)
}

func (m *Terminals) start(title, cwd, command string, args []string, overrides map[string]string, task bool, columns, rows int) (*TerminalInfo, error) {
	if command == "" {
		return nil, fmt.Errorf("No shell was found")
	}
	path, err := exec.LookPath(command)
	if err != nil {
		return nil, fmt.Errorf("%s is not installed or is missing from PATH", command)
	}
	info, err := os.Stat(cwd)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("The terminal working directory does not exist")
	}
	m.mu.Lock()
	if len(m.sessions) >= 24 {
		m.mu.Unlock()
		return nil, fmt.Errorf("Close an existing terminal before creating another")
	}
	console, err := pty.New()
	if err != nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("Could not open a terminal: %w", err)
	}
	if columns < 2 {
		columns = 100
	}
	if rows < 2 {
		rows = 24
	}
	_ = console.Resize(columns, rows)
	cmd := console.Command(path, args...)
	cmd.Dir = cwd
	environment := map[string]string{}
	for _, value := range os.Environ() {
		key, value, ok := strings.Cut(value, "=")
		if ok {
			environment[key] = value
		}
	}
	environment["TERM"] = "xterm-256color"
	environment["COLORTERM"] = "truecolor"
	environment["TERM_PROGRAM"] = "PraewaCode"
	for key, value := range overrides {
		environment[key] = value
	}
	for key, value := range environment {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	if err := cmd.Start(); err != nil {
		console.Close()
		m.mu.Unlock()
		return nil, err
	}
	if unixConsole, ok := console.(pty.UnixPty); ok {
		_ = unixConsole.Slave().Close()
	}
	id := fmt.Sprintf("terminal-%d", m.next.Add(1))
	session := &terminalSession{info: TerminalInfo{ID: id, Title: title, Cwd: cwd, Shell: path, PID: cmd.Process.Pid, Task: task}, pty: console, cmd: cmd, done: make(chan struct{}), chunks: []TerminalEvent{}}
	m.sessions[id] = session
	m.mu.Unlock()
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		buffer := make([]byte, 8192)
		for {
			n, err := console.Read(buffer)
			if n > 0 {
				session.mu.Lock()
				session.sequence++
				event := TerminalEvent{ID: id, Sequence: session.sequence, Data: base64.StdEncoding.EncodeToString(buffer[:n])}
				session.chunks = append(session.chunks, event)
				session.bytes += n
				for session.bytes > 1024*1024 && len(session.chunks) > 1 {
					session.bytes -= len(session.chunks[0].Data) * 3 / 4
					session.chunks = session.chunks[1:]
				}
				session.mu.Unlock()
				if m.OnEvent != nil {
					m.OnEvent(event)
				}
				if task && m.OnTaskOutput != nil {
					m.OnTaskOutput(id, string(buffer[:n]))
				}
			}
			if err != nil {
				return
			}
		}
	}()
	go func() {
		_ = cmd.Wait()
		select {
		case <-readDone:
		case <-time.After(time.Second):
		}
		session.closeOnce.Do(func() { _ = console.Close() })
		session.mu.Lock()
		session.info.Exited = true
		session.info.ExitCode = -1
		if cmd.ProcessState != nil {
			session.info.ExitCode = cmd.ProcessState.ExitCode()
		}
		session.sequence++
		event := TerminalEvent{ID: id, Sequence: session.sequence, Exited: true, ExitCode: session.info.ExitCode}
		session.mu.Unlock()
		if m.OnEvent != nil {
			m.OnEvent(event)
		}
		close(session.done)
	}()
	session.mu.Lock()
	copy := session.info
	session.mu.Unlock()
	return &copy, nil
}

func (m *Terminals) get(id string) (*terminalSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	session := m.sessions[id]
	if session == nil {
		return nil, fmt.Errorf("This terminal session is closed")
	}
	return session, nil
}

func (m *Terminals) Snapshot(id string) (*TerminalSnapshot, error) {
	session, err := m.get(id)
	if err != nil {
		return nil, err
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	return &TerminalSnapshot{Info: session.info, Chunks: append([]TerminalEvent{}, session.chunks...), Sequence: session.sequence}, nil
}

func (m *Terminals) Write(id, data string) error {
	if len(data) > 1024*1024 {
		return fmt.Errorf("Terminal input is too large")
	}
	session, err := m.get(id)
	if err != nil {
		return err
	}
	session.writeMu.Lock()
	defer session.writeMu.Unlock()
	session.mu.Lock()
	exited := session.info.Exited
	session.mu.Unlock()
	if exited {
		return fmt.Errorf("The terminal process has exited")
	}
	_, err = session.pty.Write([]byte(data))
	return err
}

func (m *Terminals) Resize(id string, columns, rows int) error {
	if columns < 2 || rows < 2 || columns > 1000 || rows > 500 {
		return fmt.Errorf("Invalid terminal size")
	}
	session, err := m.get(id)
	if err != nil {
		return err
	}
	return session.pty.Resize(columns, rows)
}

func (m *Terminals) List() []TerminalInfo {
	m.mu.Lock()
	defer m.mu.Unlock()
	infos := []TerminalInfo{}
	for _, session := range m.sessions {
		session.mu.Lock()
		infos = append(infos, session.info)
		session.mu.Unlock()
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].PID < infos[j].PID })
	return infos
}

func (m *Terminals) Stop(id string) error {
	session, err := m.get(id)
	if err != nil {
		return err
	}
	session.once.Do(func() {
		session.closeOnce.Do(func() { _ = session.pty.Close() })
		if session.cmd.Process != nil {
			_ = killTerminalProcess(session.cmd)
		}
	})
	return nil
}

func (m *Terminals) Close(id string) error {
	if err := m.Stop(id); err != nil {
		return err
	}
	m.mu.Lock()
	delete(m.sessions, id)
	m.mu.Unlock()
	return nil
}

func (m *Terminals) CloseAll() {
	for _, session := range m.List() {
		_ = m.Close(session.ID)
	}
}

func ShellQuote(value, shell string) string {
	if runtime.GOOS == "windows" && strings.HasPrefix(strings.ToLower(filepath.Base(shell)), "cmd") {
		return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
	}
	if runtime.GOOS == "windows" {
		return "'" + strings.ReplaceAll(value, "'", "''") + "'"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
