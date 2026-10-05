package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const modelID = "gpt-6.1-sol"
const reasoningEffort = "xhigh"

type Service struct {
	connectMu  sync.Mutex
	sendMu     sync.Mutex
	mu         sync.Mutex
	client     *rpc
	connection Connection
	chats      map[string]*Conversation
	lastEvent  map[string]time.Time
	deltaTimer map[string]*time.Timer
	root       func() string
	OnEvent    func(Event)
}

func New(root func() string) *Service {
	return &Service{root: root, chats: make(map[string]*Conversation), lastEvent: make(map[string]time.Time), deltaTimer: make(map[string]*time.Timer), connection: Connection{Models: []Model{}}}
}

func (s *Service) cwd() string {
	if root := s.root(); root != "" {
		return root
	}
	cwd, _ := os.Getwd()
	return cwd
}

func (s *Service) Status() Connection {
	s.mu.Lock()
	defer s.mu.Unlock()
	status := s.connection
	status.Models = append([]Model{}, status.Models...)
	return status
}

func (s *Service) Active() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, chat := range s.chats {
		if chat.Active && chat.CanStop {
			return true
		}
	}
	return false
}

func (s *Service) Connect() (Connection, error) {
	s.connectMu.Lock()
	defer s.connectMu.Unlock()
	s.mu.Lock()
	client := s.client
	if client != nil {
		select {
		case <-client.done:
			client = nil
			s.client = nil
		default:
		}
	}
	s.mu.Unlock()
	if client != nil && s.Status().Connected {
		return s.Status(), nil
	}
	if client == nil {
		path, err := executable()
		if err != nil {
			return s.failedConnection(err)
		}
		args := []string{"app-server", "--listen", "stdio://"}
		transport := "local"
		if info, err := os.Stat(socketPath()); err == nil && info.Mode()&os.ModeSocket != 0 {
			args = []string{"app-server", "proxy", "--sock", socketPath()}
			transport = "shared"
		}
		client, err = startRPC(path, args, s.cwd(), s.handle, s.disconnected)
		if err != nil {
			return s.failedConnection(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		err = client.request(ctx, "initialize", map[string]any{"clientInfo": map[string]string{"name": "praewa_code", "title": "Praewa Code", "version": "0.3.0"}}, nil)
		cancel()
		if err != nil {
			client.close()
			return s.failedConnection(fmt.Errorf("Could not connect to Codex: %w", err))
		}
		if err := client.write(map[string]any{"method": "initialized", "params": map[string]any{}}); err != nil {
			client.close()
			return s.failedConnection(err)
		}
		s.mu.Lock()
		s.client = client
		s.connection.Transport = transport
		s.mu.Unlock()
	}
	var account struct {
		Account *struct {
			Type string `json:"type"`
		} `json:"account"`
		RequiresAuth bool `json:"requiresOpenaiAuth"`
	}
	if err := s.call("account/read", map[string]any{"refreshToken": false}, &account); err != nil {
		return s.failedConnection(err)
	}
	if account.Account == nil && account.RequiresAuth {
		return s.failedConnection(errors.New("Sign in to ChatGPT in Codex, then reconnect here"))
	}
	var models struct {
		Data []struct {
			Model       string `json:"model"`
			DisplayName string `json:"displayName"`
			Description string `json:"description"`
			Efforts     []struct {
				Effort string `json:"reasoningEffort"`
			} `json:"supportedReasoningEfforts"`
		} `json:"data"`
	}
	if err := s.call("model/list", map[string]any{"limit": 100}, &models); err != nil {
		return s.failedConnection(err)
	}
	var available *Model
	for _, model := range models.Data {
		if model.Model != modelID {
			continue
		}
		for _, effort := range model.Efforts {
			if effort.Effort == reasoningEffort {
				available = &Model{ID: modelID, Name: model.DisplayName, Description: model.Description, Default: true}
				break
			}
		}
	}
	if available == nil {
		return s.failedConnection(errors.New("The configured agent is unavailable for this account. Check your account access and try again"))
	}
	s.mu.Lock()
	s.connection.Connected = true
	s.connection.Account = "Codex"
	if account.Account != nil && account.Account.Type == "chatgpt" {
		s.connection.Account = "ChatGPT"
	}
	s.connection.Error = ""
	s.connection.Models = []Model{*available}
	s.mu.Unlock()
	return s.Status(), nil
}

func (s *Service) failedConnection(err error) (Connection, error) {
	s.mu.Lock()
	s.connection.Connected = false
	s.connection.Error = err.Error()
	s.mu.Unlock()
	return s.Status(), err
}

func (s *Service) call(method string, params any, result any) error {
	s.mu.Lock()
	client := s.client
	s.mu.Unlock()
	if client == nil {
		return errors.New("Connect to Codex first")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	return client.request(ctx, method, params, result)
}

func (s *Service) Threads(cursor string) (ThreadPage, error) {
	page := ThreadPage{Data: []Thread{}}
	params := map[string]any{"cwd": s.cwd(), "limit": 30, "sortKey": "updated_at", "sourceKinds": []string{"cli", "vscode", "appServer", "exec"}, "useStateDbOnly": true}
	if cursor != "" {
		params["cursor"] = cursor
	}
	err := s.call("thread/list", params, &page)
	return page, err
}

func (s *Service) readThread(id string) (Thread, error) {
	var result struct {
		Thread Thread `json:"thread"`
	}
	if err := s.call("thread/read", map[string]any{"threadId": id, "includeTurns": true}, &result); err != nil {
		return Thread{}, err
	}
	if filepath.Clean(result.Thread.Cwd) != filepath.Clean(s.cwd()) {
		return Thread{}, errors.New("This Codex chat belongs to another project. Open its workspace first")
	}
	return result.Thread, nil
}

func fromThread(thread Thread) *Conversation {
	chat := &Conversation{Thread: thread, Messages: []Message{}, Requests: []Request{}, Active: thread.Status.Type == "active"}
	for _, turn := range thread.Turns {
		for _, entry := range turn.Items {
			if message := itemMessage(entry, turn.ID, turn.StartedAt); message != nil {
				message.Status = turn.Status
				chat.Messages = append(chat.Messages, *message)
			}
		}
		if turn.Status == "inProgress" {
			chat.Active = true
			chat.TurnID = turn.ID
		}
	}
	if len(chat.Messages) > 120 {
		chat.Messages = chat.Messages[len(chat.Messages)-120:]
	}
	chat.Thread.Turns = nil
	return chat
}

func (s *Service) Load(id string) (Conversation, error) {
	thread, err := s.readThread(id)
	if err != nil {
		return Conversation{}, err
	}
	s.mu.Lock()
	if old := s.chats[id]; old != nil && old.CanStop {
		result := copyConversation(old)
		s.mu.Unlock()
		return result, nil
	}
	chat := fromThread(thread)
	if old := s.chats[id]; old != nil {
		chat.Sequence = old.Sequence + 1
	}
	s.chats[id] = chat
	result := copyConversation(chat)
	s.mu.Unlock()
	return result, nil
}

func copyConversation(chat *Conversation) Conversation {
	copy := *chat
	copy.Messages = append([]Message{}, chat.Messages...)
	copy.Requests = append([]Request{}, chat.Requests...)
	copy.Thread.Turns = nil
	return copy
}

func (s *Service) Snapshot(id string) (Conversation, error) {
	s.mu.Lock()
	chat := s.chats[id]
	if chat != nil && filepath.Clean(chat.Thread.Cwd) == filepath.Clean(s.cwd()) {
		result := copyConversation(chat)
		s.mu.Unlock()
		return result, nil
	}
	s.mu.Unlock()
	return s.Load(id)
}

func (s *Service) Send(request SendRequest) (Conversation, error) {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	if !s.Status().Connected {
		if _, err := s.Connect(); err != nil {
			return Conversation{}, err
		}
	}
	request.Text = strings.TrimSpace(request.Text)
	if request.Text == "" || len(request.Text) > 80000 {
		return Conversation{}, errors.New("Enter a message under 20,000 characters")
	}
	contextText, err := s.contextText(request)
	if err != nil {
		return Conversation{}, err
	}
	id := request.ThreadID
	if id == "" {
		params := map[string]any{"cwd": s.cwd(), "model": modelID, "config": map[string]any{"model_reasoning_effort": reasoningEffort}, "approvalPolicy": "on-request", "sandbox": "read-only", "developerInstructions": developerInstructions(request.Mode)}
		if request.Mode.Behavior == "code" && s.root() != "" {
			params["sandbox"] = "workspace-write"
		}
		var result struct {
			Thread Thread `json:"thread"`
		}
		if err := s.call("thread/start", params, &result); err != nil {
			return Conversation{}, err
		}
		id = result.Thread.ID
		s.mu.Lock()
		s.chats[id] = fromThread(result.Thread)
		s.mu.Unlock()
	} else {
		chat, err := s.Load(id)
		if err != nil {
			return Conversation{}, err
		}
		if chat.Active {
			return Conversation{}, errors.New("This Codex chat is still working. Wait for it to finish before sending")
		}
		if err := s.call("thread/resume", map[string]any{"threadId": id, "model": modelID, "config": map[string]any{"model_reasoning_effort": reasoningEffort}, "developerInstructions": developerInstructions(request.Mode)}, nil); err != nil {
			return Conversation{}, err
		}
	}
	input := []map[string]any{{"type": "text", "text": request.Text}}
	if contextText != "" {
		input = append(input, map[string]any{"type": "text", "text": contextText})
	}
	policy := map[string]any{"type": "readOnly"}
	if request.Mode.Behavior == "code" && s.root() != "" {
		policy = map[string]any{"type": "workspaceWrite", "writableRoots": []string{s.cwd()}, "networkAccess": false}
	}
	params := map[string]any{"threadId": id, "input": input, "model": modelID, "effort": reasoningEffort, "sandboxPolicy": policy, "approvalPolicy": "on-request", "approvalsReviewer": "user"}
	if request.ID != "" {
		params["clientUserMessageId"] = request.ID
	}
	var result struct {
		Turn turn `json:"turn"`
	}
	s.mu.Lock()
	chat := s.chats[id]
	chat.Active = true
	chat.CanStop = true
	chat.Error = ""
	chat.Sequence++
	s.mu.Unlock()
	if err := s.call("turn/start", params, &result); err != nil {
		s.mu.Lock()
		chat.Active = false
		chat.CanStop = false
		chat.Error = err.Error()
		chat.Sequence++
		s.mu.Unlock()
		snapshot, _ := s.Snapshot(id)
		return snapshot, err
	}
	s.mu.Lock()
	chat.TurnID = result.Turn.ID
	chat.Sequence++
	s.mu.Unlock()
	return s.Snapshot(id)
}

func (s *Service) contextText(request SendRequest) (string, error) {
	if request.Mode.Behavior != "code" && request.Mode.Behavior != "plan" && request.Mode.Behavior != "ask" && request.Mode.Behavior != "review" && request.Mode.Behavior != "chat" {
		return "", errors.New("Choose a Code, Plan, Ask, Review, or Chat behavior for this mode")
	}
	var text strings.Builder
	fmt.Fprintf(&text, "<praewa_context>\nSelected mode: %s\nBehavior: %s\n", request.Mode.Name, request.Mode.Behavior)
	behavior := map[string]string{"code": "Use the selected persona to decide whether to accept a coding request. When it accepts, implement the work and verify it. When it refuses, respond in character without modifying files.", "plan": "Prepare a plan. Do not modify files.", "ask": "Explain and answer questions using the selected persona. Do not modify files.", "review": "Review for bugs and regressions. Do not modify files.", "chat": "Only converse in the selected persona. Do not perform tasks, use tools, read files, run commands or modify files."}[request.Mode.Behavior]
	fmt.Fprintln(&text, behavior)
	fmt.Fprintln(&text, request.Mode.Instructions)
	for _, skill := range request.Mode.Skills {
		if len(skill.Content) > 256*1024 {
			return "", fmt.Errorf("Skill %s is too large", skill.Name)
		}
		fmt.Fprintf(&text, "\nSkill file: %s\n%s\n", skill.Name, skill.Content)
	}
	for _, file := range request.Files {
		if !strings.HasPrefix(file.Path, "untitled:") {
			root := s.root()
			if root == "" {
				return "", errors.New("Open a workspace before attaching project files")
			}
			path := file.Path
			if !filepath.IsAbs(path) {
				path = filepath.Join(root, path)
			}
			realRoot, err := filepath.EvalSymlinks(root)
			if err != nil {
				return "", err
			}
			real, err := filepath.EvalSymlinks(path)
			if err != nil {
				return "", err
			}
			relative, err := filepath.Rel(realRoot, real)
			if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
				return "", errors.New("Attached files must be inside the current workspace")
			}
		}
		if len(file.Content) > 256*1024 {
			return "", fmt.Errorf("%s is too large to attach. Choose a file under 256 KB", file.Name)
		}
		fmt.Fprintf(&text, "\nAttached file: %s (%s)\n%s\n", file.Name, file.Path, file.Content)
	}
	fmt.Fprintln(&text, "</praewa_context>")
	if text.Len() > 1024*1024 {
		return "", errors.New("The selected skills and files are too large. Attach fewer files")
	}
	return text.String(), nil
}

func developerInstructions(mode Mode) string {
	var text strings.Builder
	fmt.Fprintln(&text, "You are the agent in Praewa Code. The user selected the mode below for the current turn. Follow its persona, behavior and skills, including a persona that refuses work. The current selection replaces any previously selected mode; do not carry over the earlier persona. Answer in the user's language unless the selected mode or skill specifies another language. Do not add code comments unless requested.")
	fmt.Fprintf(&text, "\n<selected_mode>\nName: %s\nBehavior: %s\n%s\n", mode.Name, mode.Behavior, mode.Instructions)
	for _, skill := range mode.Skills {
		fmt.Fprintf(&text, "\nSkill file: %s\n%s\n", skill.Name, skill.Content)
	}
	fmt.Fprintln(&text, "</selected_mode>")
	return text.String()
}

func (s *Service) Stop(id string) error {
	chat, err := s.Snapshot(id)
	if err != nil {
		return err
	}
	if !chat.CanStop || chat.TurnID == "" {
		return errors.New("There is no turn from this panel to stop")
	}
	return s.call("turn/interrupt", map[string]any{"threadId": id, "turnId": chat.TurnID}, nil)
}

func itemMessage(entry item, turnID string, createdAt int64) *Message {
	message := &Message{ID: entry.ID, TurnID: turnID, CreatedAt: createdAt, Status: "inProgress", Phase: entry.Phase}
	switch entry.Type {
	case "userMessage":
		message.Role = "user"
		for _, content := range entry.Content {
			if content.Type == "text" {
				message.Text = content.Text
				break
			}
		}
	case "agentMessage", "plan":
		message.Role = "assistant"
		message.Text = entry.Text
	default:
		return nil
	}
	return message
}

func upsert(chat *Conversation, message Message) {
	for i := range chat.Messages {
		if chat.Messages[i].ID == message.ID {
			chat.Messages[i] = message
			return
		}
	}
	chat.Messages = append(chat.Messages, message)
}

func (s *Service) handle(message envelope) {
	firstDelta := false
	var params struct {
		ThreadID  string          `json:"threadId"`
		TurnID    string          `json:"turnId"`
		ItemID    string          `json:"itemId"`
		RequestID json.RawMessage `json:"requestId"`
		Delta     string          `json:"delta"`
		Turn      turn            `json:"turn"`
		Item      item            `json:"item"`
		Error     struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(message.Params, &params) != nil {
		return
	}
	s.mu.Lock()
	chat := s.chats[params.ThreadID]
	if chat == nil {
		s.mu.Unlock()
		if len(message.ID) != 0 {
			s.rejectUnsupported(message)
		}
		return
	}
	if len(message.ID) != 0 {
		request := parseRequest(message, params.ThreadID, params.TurnID)
		if request == nil {
			s.mu.Unlock()
			s.rejectUnsupported(message)
			return
		}
		chat.Requests = append(chat.Requests, *request)
	} else {
		switch message.Method {
		case "turn/started":
			chat.Active = true
			chat.TurnID = params.Turn.ID
			chat.Error = ""
		case "item/started", "item/completed":
			if next := itemMessage(params.Item, params.TurnID, time.Now().Unix()); next != nil {
				if message.Method == "item/completed" {
					next.Status = "completed"
				}
				upsert(chat, *next)
			}
		case "item/agentMessage/delta":
			found := false
			for i := range chat.Messages {
				if chat.Messages[i].ID == params.ItemID {
					firstDelta = chat.Messages[i].Text == ""
					chat.Messages[i].Text += params.Delta
					found = true
					break
				}
			}
			if !found {
				firstDelta = true
				upsert(chat, Message{ID: params.ItemID, TurnID: params.TurnID, Role: "assistant", Text: params.Delta, Status: "inProgress", CreatedAt: time.Now().Unix()})
			}
		case "turn/completed":
			chat.Active = false
			chat.CanStop = false
			chat.Requests = []Request{}
			chat.TurnID = params.Turn.ID
			if params.Turn.Error != nil {
				chat.Error = params.Turn.Error.Message
			}
			for i := range chat.Messages {
				if chat.Messages[i].TurnID == params.Turn.ID {
					chat.Messages[i].Status = params.Turn.Status
				}
			}
		case "error":
			chat.Error = params.Error.Message
		case "serverRequest/resolved":
			filtered := chat.Requests[:0]
			for _, request := range chat.Requests {
				if request.ID != string(params.RequestID) {
					filtered = append(filtered, request)
				}
			}
			chat.Requests = filtered
		default:
			s.mu.Unlock()
			return
		}
	}
	chat.Sequence++
	if s.OnEvent == nil {
		s.mu.Unlock()
		return
	}
	if message.Method == "item/agentMessage/delta" && !firstDelta && time.Since(s.lastEvent[params.ThreadID]) < 50*time.Millisecond {
		id := params.ThreadID
		if s.deltaTimer[id] == nil {
			s.deltaTimer[id] = time.AfterFunc(50*time.Millisecond, func() {
				s.mu.Lock()
				delete(s.deltaTimer, id)
				s.lastEvent[id] = time.Now()
				snapshot := copyConversation(s.chats[id])
				s.mu.Unlock()
				s.emit(snapshot)
			})
		}
		s.mu.Unlock()
		return
	}
	if timer := s.deltaTimer[params.ThreadID]; timer != nil {
		timer.Stop()
		delete(s.deltaTimer, params.ThreadID)
	}
	s.lastEvent[params.ThreadID] = time.Now()
	result := copyConversation(chat)
	s.mu.Unlock()
	s.emit(result)
}

func (s *Service) emit(chat Conversation) {
	if s.OnEvent != nil {
		s.OnEvent(Event{ThreadID: chat.Thread.ID, Snapshot: &chat})
	}
}

func (s *Service) disconnected(source *rpc, reason error) {
	s.mu.Lock()
	if s.client != source {
		s.mu.Unlock()
		return
	}
	s.client = nil
	s.connection.Connected = false
	s.connection.Error = "Codex disconnected. Reconnect to continue."
	var changed []Conversation
	for _, chat := range s.chats {
		if chat.Active {
			chat.Active = false
			chat.CanStop = false
			chat.Requests = []Request{}
			chat.Error = s.connection.Error
			chat.Sequence++
			changed = append(changed, copyConversation(chat))
		}
	}
	s.mu.Unlock()
	for _, chat := range changed {
		s.emit(chat)
	}
}

func (s *Service) Close() {
	s.connectMu.Lock()
	defer s.connectMu.Unlock()
	s.mu.Lock()
	client := s.client
	s.client = nil
	for id, timer := range s.deltaTimer {
		timer.Stop()
		delete(s.deltaTimer, id)
	}
	s.mu.Unlock()
	if client != nil {
		client.close()
	}
}
