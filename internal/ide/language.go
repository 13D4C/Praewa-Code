package ide

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type LanguageStatus struct {
	Language  string `json:"language"`
	Server    string `json:"server"`
	Available bool   `json:"available"`
	Version   int    `json:"version"`
	Problem   string `json:"problem"`
}

type LanguageDiagnostics struct {
	Path        string            `json:"path"`
	Version     int               `json:"version"`
	Diagnostics []json.RawMessage `json:"diagnostics"`
}

type languageDocument struct {
	content string
	version int
}

type languageResponse struct {
	result json.RawMessage
	err    error
}

type languageClient struct {
	mu            sync.Mutex
	writeMu       sync.Mutex
	docMu         sync.Mutex
	cmd           *exec.Cmd
	input         io.WriteCloser
	pending       map[uint64]chan languageResponse
	documents     map[string]languageDocument
	next          atomic.Uint64
	done          chan struct{}
	once          sync.Once
	capabilities  map[string]json.RawMessage
	onDiagnostics func(LanguageDiagnostics)
}

type Languages struct {
	mu            sync.Mutex
	workspace     *Workspace
	clients       map[string]*languageClient
	failures      map[string]string
	OnDiagnostics func(LanguageDiagnostics)
}

func NewLanguages(w *Workspace) *Languages {
	return &Languages{workspace: w, clients: map[string]*languageClient{}, failures: map[string]string{}}
}

func languageServer(path string) (string, string, []string) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "go", "gopls", []string{"serve"}
	case ".rs":
		return "rust", "rust-analyzer", nil
	case ".py":
		return "python", "pylsp", nil
	case ".tsx":
		return "typescriptreact", "typescript-language-server", []string{"--stdio"}
	case ".ts":
		return "typescript", "typescript-language-server", []string{"--stdio"}
	case ".jsx":
		return "javascriptreact", "typescript-language-server", []string{"--stdio"}
	case ".js", ".mjs", ".cjs":
		return "javascript", "typescript-language-server", []string{"--stdio"}
	case ".c", ".h":
		return "c", "clangd", nil
	case ".cpp", ".hpp", ".cc":
		return "cpp", "clangd", nil
	case ".svelte":
		return "svelte", "svelteserver", []string{"--stdio"}
	}
	return "", "", nil
}

func fileURI(path string) string {
	path = filepath.ToSlash(path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return (&url.URL{Scheme: "file", Path: path}).String()
}

func uriFile(uri string) string {
	parsed, err := url.Parse(uri)
	if err != nil || parsed.Scheme != "file" {
		return ""
	}
	path := parsed.Path
	if len(path) > 3 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}
	return filepath.FromSlash(path)
}

func (l *Languages) server(path string) (*languageClient, LanguageStatus) {
	language, name, args := languageServer(path)
	status := LanguageStatus{Language: language, Server: name}
	if name == "" {
		return nil, status
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if client := l.clients[name]; client != nil {
		select {
		case <-client.done:
			delete(l.clients, name)
			status.Problem = "Language server stopped. Reopen the workspace to restart it"
			l.failures[name] = status.Problem
			return nil, status
		default:
			status.Available = true
			return client, status
		}
	}
	if failure := l.failures[name]; failure != "" {
		status.Problem = failure
		return nil, status
	}
	command, err := exec.LookPath(name)
	if err != nil {
		for dir := filepath.Dir(path); inside(l.workspace.Root(), dir); dir = filepath.Dir(dir) {
			candidate := filepath.Join(dir, "node_modules", ".bin", name)
			if file, err := os.Stat(candidate); err == nil && !file.IsDir() {
				command = candidate
				break
			}
			if dir == l.workspace.Root() {
				break
			}
		}
	}
	if command == "" {
		status.Problem = name + " is not installed or is missing from PATH"
		return nil, status
	}
	client, err := startLanguageClient(command, args, l.workspace.Root(), l.OnDiagnostics)
	if err != nil {
		status.Problem = err.Error()
		l.failures[name] = status.Problem
		return nil, status
	}
	l.clients[name] = client
	status.Available = true
	return client, status
}

func startLanguageClient(command string, args []string, root string, onDiagnostics func(LanguageDiagnostics)) (*languageClient, error) {
	cmd := exec.Command(command, args...)
	cmd.Dir = root
	input, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	client := &languageClient{cmd: cmd, input: input, pending: map[uint64]chan languageResponse{}, documents: map[string]languageDocument{}, done: make(chan struct{}), onDiagnostics: onDiagnostics}
	go func() { _, _ = io.Copy(io.Discard, stderr) }()
	go client.read(output)
	go func() { _ = cmd.Wait(); client.finish() }()
	result, err := client.request("initialize", map[string]any{"processId": os.Getpid(), "rootUri": fileURI(root), "workspaceFolders": []map[string]string{{"uri": fileURI(root), "name": filepath.Base(root)}}, "clientInfo": map[string]string{"name": "Praewa Code", "version": "0.2.0"}, "capabilities": map[string]any{"general": map[string]any{"positionEncodings": []string{"utf-16"}}, "textDocument": map[string]any{"synchronization": map[string]any{"dynamicRegistration": false, "didSave": true}, "completion": map[string]any{"completionItem": map[string]any{"snippetSupport": false}}, "hover": map[string]any{"contentFormat": []string{"plaintext"}}, "publishDiagnostics": map[string]any{"versionSupport": true}}}}, 25*time.Second)
	if err != nil {
		client.close()
		return nil, fmt.Errorf("%s: %w", filepath.Base(command), err)
	}
	var initialize struct {
		Capabilities map[string]json.RawMessage `json:"capabilities"`
	}
	_ = json.Unmarshal(result, &initialize)
	client.capabilities = initialize.Capabilities
	_ = client.notify("initialized", map[string]any{})
	return client, nil
}

func (c *languageClient) finish() {
	c.once.Do(func() {
		close(c.done)
		c.mu.Lock()
		defer c.mu.Unlock()
		for id, ch := range c.pending {
			ch <- languageResponse{err: fmt.Errorf("Language server stopped")}
			delete(c.pending, id)
		}
	})
}

func (c *languageClient) close() {
	_ = c.input.Close()
	if c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	c.finish()
}

func (c *languageClient) send(payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	select {
	case <-c.done:
		return fmt.Errorf("Language server stopped")
	default:
	}
	_, err = fmt.Fprintf(c.input, "Content-Length: %d\r\n\r\n%s", len(data), data)
	return err
}
func (c *languageClient) notify(method string, params any) error {
	return c.send(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}
func (c *languageClient) request(method string, params any, timeout time.Duration) (json.RawMessage, error) {
	id := c.next.Add(1)
	ch := make(chan languageResponse, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()
	defer func() { c.mu.Lock(); delete(c.pending, id); c.mu.Unlock() }()
	if err := c.send(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		return nil, err
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case response := <-ch:
		return response.result, response.err
	case <-c.done:
		return nil, fmt.Errorf("Language server stopped")
	case <-timer.C:
		_ = c.notify("$/cancelRequest", map[string]uint64{"id": id})
		return nil, fmt.Errorf("Language server request timed out")
	}
}
func (c *languageClient) read(output io.Reader) {
	reader := bufio.NewReader(output)
	defer c.close()
	for {
		length := 0
		headerBytes := 0
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			headerBytes += len(line)
			if headerBytes > 8192 {
				return
			}
			if strings.TrimSpace(line) == "" {
				break
			}
			key, value, _ := strings.Cut(line, ":")
			if strings.EqualFold(key, "Content-Length") {
				length, _ = strconv.Atoi(strings.TrimSpace(value))
			}
		}
		if length <= 0 || length > 20*1024*1024 {
			return
		}
		data := make([]byte, length)
		if _, err := io.ReadFull(reader, data); err != nil {
			return
		}
		var response struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(data, &response) != nil {
			continue
		}
		if response.Method != "" {
			if len(response.ID) > 0 {
				var result any
				switch response.Method {
				case "workspace/configuration":
					var params struct {
						Items []any `json:"items"`
					}
					_ = json.Unmarshal(response.Params, &params)
					result = make([]any, len(params.Items))
				case "workspace/applyEdit":
					result = map[string]any{"applied": false, "failureReason": "Apply changes through the editor"}
				case "workspace/workspaceFolders":
					result = []any{}
				}
				_ = c.send(map[string]any{"jsonrpc": "2.0", "id": response.ID, "result": result})
			} else if response.Method == "textDocument/publishDiagnostics" {
				var params struct {
					URI         string            `json:"uri"`
					Version     int               `json:"version"`
					Diagnostics []json.RawMessage `json:"diagnostics"`
				}
				_ = json.Unmarshal(response.Params, &params)
				path := uriFile(params.URI)
				if path != "" && c.onDiagnostics != nil {
					c.onDiagnostics(LanguageDiagnostics{Path: path, Version: params.Version, Diagnostics: params.Diagnostics})
				}
			}
			continue
		}
		var id uint64
		if json.Unmarshal(response.ID, &id) != nil {
			continue
		}
		c.mu.Lock()
		ch := c.pending[id]
		delete(c.pending, id)
		c.mu.Unlock()
		if ch != nil {
			result := languageResponse{result: response.Result}
			if response.Error != nil {
				result.err = fmt.Errorf("%s", response.Error.Message)
			}
			ch <- result
		}
	}
}

func (l *Languages) Sync(path, content string) LanguageStatus {
	if len(content) > maxFileSize {
		return LanguageStatus{Problem: "Files must be smaller than 5 MB"}
	}
	resolved, err := l.workspace.resolve(path, false)
	if err != nil {
		return LanguageStatus{Problem: err.Error()}
	}
	client, status := l.server(resolved)
	if client == nil {
		return status
	}
	client.docMu.Lock()
	defer client.docMu.Unlock()
	previous, exists := client.documents[resolved]
	version := previous.version
	if !exists || previous.content != content {
		version++
		client.documents[resolved] = languageDocument{content: content, version: version}
		if exists {
			err = client.notify("textDocument/didChange", map[string]any{"textDocument": map[string]any{"uri": fileURI(resolved), "version": version}, "contentChanges": []map[string]string{{"text": content}}})
		} else {
			err = client.notify("textDocument/didOpen", map[string]any{"textDocument": map[string]any{"uri": fileURI(resolved), "languageId": status.Language, "version": version, "text": content}})
		}
	}
	status.Version = version
	if err != nil {
		status.Problem = err.Error()
		status.Available = false
	}
	return status
}

func (l *Languages) Request(path, content, method string, line, character int) (json.RawMessage, error) {
	allowed := map[string]string{"completion": "completionProvider", "hover": "hoverProvider", "definition": "definitionProvider", "documentSymbol": "documentSymbolProvider", "formatting": "documentFormattingProvider"}
	capability, ok := allowed[method]
	if !ok {
		return nil, fmt.Errorf("Unsupported language request")
	}
	resolved, err := l.workspace.resolve(path, false)
	if err != nil {
		return nil, err
	}
	status := l.Sync(resolved, content)
	if !status.Available {
		return json.RawMessage("null"), nil
	}
	client, _ := l.server(resolved)
	if client == nil {
		return json.RawMessage("null"), nil
	}
	if value := client.capabilities[capability]; len(value) == 0 || string(value) == "false" {
		return json.RawMessage("null"), nil
	}
	params := map[string]any{"textDocument": map[string]string{"uri": fileURI(resolved)}}
	if method == "formatting" {
		params["options"] = map[string]any{"tabSize": 4, "insertSpaces": true}
	} else if method != "documentSymbol" {
		params["position"] = map[string]int{"line": line, "character": character}
	}
	return client.request("textDocument/"+method, params, 8*time.Second)
}
func (l *Languages) Saved(path string) {
	_, name, _ := languageServer(path)
	l.mu.Lock()
	client := l.clients[name]
	l.mu.Unlock()
	if client != nil {
		_ = client.notify("textDocument/didSave", map[string]any{"textDocument": map[string]string{"uri": fileURI(path)}})
	}
}
func (l *Languages) CloseDocument(path string) {
	_, name, _ := languageServer(path)
	l.mu.Lock()
	client := l.clients[name]
	l.mu.Unlock()
	if client != nil {
		client.docMu.Lock()
		delete(client.documents, path)
		_ = client.notify("textDocument/didClose", map[string]any{"textDocument": map[string]string{"uri": fileURI(path)}})
		client.docMu.Unlock()
	}
}
func (l *Languages) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, client := range l.clients {
		client.close()
	}
	l.clients = map[string]*languageClient{}
	l.failures = map[string]string{}
}
