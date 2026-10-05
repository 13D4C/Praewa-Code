package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

type envelope struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpc struct {
	cmd       *exec.Cmd
	in        io.WriteCloser
	mu        sync.Mutex
	writeMu   sync.Mutex
	pending   map[string]chan envelope
	next      atomic.Int64
	done      chan struct{}
	closeOnce sync.Once
	onMessage func(envelope)
	onClose   func(*rpc, error)
}

func executable() (string, error) {
	if path := os.Getenv("PRAEWA_CODEX_PATH"); path != "" {
		return exec.LookPath(path)
	}
	if path, err := exec.LookPath("codex"); err == nil {
		return path, nil
	}
	var candidates []string
	switch runtime.GOOS {
	case "linux":
		candidates = []string{"/usr/lib/chatgpt/resources/codex", "/usr/lib/codex/resources/codex"}
	case "darwin":
		candidates = []string{"/Applications/ChatGPT.app/Contents/Resources/codex", "/Applications/Codex.app/Contents/Resources/codex"}
	}
	for _, path := range candidates {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, nil
		}
	}
	return "", errors.New("Codex was not found. Install Codex or set PRAEWA_CODEX_PATH to its executable")
}

func socketPath() string {
	if path := os.Getenv("PRAEWA_CODEX_SOCKET"); path != "" {
		return path
	}
	home := os.Getenv("CODEX_HOME")
	if home == "" {
		userHome, _ := os.UserHomeDir()
		home = filepath.Join(userHome, ".codex")
	}
	return filepath.Join(home, "app-server-control", "app-server-control.sock")
}

func startRPC(path string, args []string, cwd string, onMessage func(envelope), onClose func(*rpc, error)) (*rpc, error) {
	cmd := exec.Command(path, args...)
	cmd.Dir = cwd
	cmd.Env = os.Environ()
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		_ = in.Close()
		return nil, err
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		_ = in.Close()
		return nil, err
	}
	r := &rpc{cmd: cmd, in: in, pending: make(map[string]chan envelope), done: make(chan struct{}), onMessage: onMessage, onClose: onClose}
	go func() {
		readErr := r.read(out)
		if readErr != nil {
			_ = cmd.Process.Kill()
		}
		err := cmd.Wait()
		if readErr != nil {
			err = readErr
		}
		if err == nil {
			err = errors.New("Codex disconnected")
		}
		r.finish(err)
	}()
	return r, nil
}

func (r *rpc) read(reader io.Reader) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 64*1024*1024)
	for scanner.Scan() {
		var message envelope
		if json.Unmarshal(scanner.Bytes(), &message) != nil {
			continue
		}
		if message.Method != "" {
			if r.onMessage != nil {
				r.onMessage(message)
			}
			continue
		}
		r.mu.Lock()
		channel := r.pending[string(message.ID)]
		delete(r.pending, string(message.ID))
		r.mu.Unlock()
		if channel != nil {
			channel <- message
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("Codex response could not be read: %w", err)
	}
	return nil
}

func (r *rpc) finish(reason error) {
	r.closeOnce.Do(func() {
		close(r.done)
		r.mu.Lock()
		for key, channel := range r.pending {
			channel <- envelope{Error: &rpcError{Code: -32000, Message: reason.Error()}}
			delete(r.pending, key)
		}
		r.mu.Unlock()
		if r.onClose != nil {
			r.onClose(r, reason)
		}
	})
}

func (r *rpc) write(message any) error {
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}
	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	select {
	case <-r.done:
		return errors.New("Codex is disconnected. Reconnect and try again")
	default:
	}
	_, err = r.in.Write(append(data, '\n'))
	return err
}

func (r *rpc) request(ctx context.Context, method string, params any, result any) error {
	id := r.next.Add(1)
	key := strconv.FormatInt(id, 10)
	channel := make(chan envelope, 1)
	r.mu.Lock()
	select {
	case <-r.done:
		r.mu.Unlock()
		return errors.New("Codex is disconnected. Reconnect and try again")
	default:
	}
	r.pending[key] = channel
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		delete(r.pending, key)
		r.mu.Unlock()
	}()
	if err := r.write(map[string]any{"id": id, "method": method, "params": params}); err != nil {
		return err
	}
	select {
	case message := <-channel:
		if message.Error != nil {
			return errors.New(message.Error.Message)
		}
		if result == nil {
			return nil
		}
		return json.Unmarshal(message.Result, result)
	case <-ctx.Done():
		return fmt.Errorf("Codex %s: %w", method, ctx.Err())
	}
}

func (r *rpc) close() {
	_ = r.in.Close()
	select {
	case <-r.done:
		return
	case <-time.After(time.Second):
	}
	if r.cmd.Process != nil {
		_ = r.cmd.Process.Kill()
	}
}
