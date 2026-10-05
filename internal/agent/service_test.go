package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAgentHelperProcess(t *testing.T) {
	if os.Getenv("PRAEWA_AGENT_HELPER") != "1" {
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	write := func(value any) { data, _ := json.Marshal(value); fmt.Println(string(data)) }
	recorded := make(map[string][]map[string]json.RawMessage)
	modelCatalog := json.RawMessage(`[{"model":"other-model","displayName":"Other","supportedReasoningEfforts":[{"reasoningEffort":"xhigh"}]},{"model":"gpt-6.1-sol","displayName":"GPT-6.1 Sol","supportedReasoningEfforts":[{"reasoningEffort":"low"},{"reasoningEffort":"xhigh"}]}]`)
	for scanner.Scan() {
		var message struct {
			ID     json.RawMessage            `json:"id"`
			Method string                     `json:"method"`
			Params map[string]json.RawMessage `json:"params"`
			Result json.RawMessage            `json:"result"`
		}
		_ = json.Unmarshal(scanner.Bytes(), &message)
		recorded[message.Method] = append(recorded[message.Method], message.Params)
		var result any = map[string]any{}
		switch message.Method {
		case "test/settings":
			result = recorded
		case "test/models":
			modelCatalog = message.Params["data"]
		case "account/read":
			result = map[string]any{"account": map[string]string{"type": "chatgpt"}, "requiresOpenaiAuth": true}
		case "model/list":
			result = map[string]any{"data": modelCatalog}
		case "hang":
			continue
		case "echo":
			result = message.Params
		case "thread/start", "thread/read":
			cwd, _ := os.Getwd()
			result = map[string]any{"thread": map[string]any{"id": "fake-thread", "cwd": cwd, "model": "old-model", "status": map[string]any{"type": "idle"}, "turns": []any{}}}
		case "turn/start":
			var inputs []map[string]string
			_ = json.Unmarshal(message.Params["input"], &inputs)
			write(map[string]any{"method": "turn/started", "params": map[string]any{"threadId": "fake-thread", "turn": map[string]string{"id": "fake-turn"}}})
			write(map[string]any{"method": "item/completed", "params": map[string]any{"threadId": "fake-thread", "turnId": "fake-turn", "item": map[string]any{"id": "user-1", "type": "userMessage", "content": inputs}}})
			for _, delta := range []string{"Hello", " world"} {
				write(map[string]any{"method": "item/agentMessage/delta", "params": map[string]string{"threadId": "fake-thread", "turnId": "fake-turn", "itemId": "reply-1", "delta": delta}})
			}
			write(map[string]any{"method": "item/completed", "params": map[string]any{"threadId": "fake-thread", "turnId": "fake-turn", "item": map[string]string{"id": "reply-1", "type": "agentMessage", "text": "Hello world"}}})
			write(map[string]any{"method": "turn/completed", "params": map[string]any{"threadId": "fake-thread", "turn": map[string]string{"id": "fake-turn", "status": "completed"}}})
			result = map[string]any{"turn": map[string]string{"id": "fake-turn", "status": "completed"}}
		}
		if len(message.ID) != 0 {
			write(map[string]any{"id": message.ID, "result": result})
		}
	}
	os.Exit(0)
}

func fakeService(t *testing.T) (*Service, *rpc) {
	t.Helper()
	t.Setenv("PRAEWA_AGENT_HELPER", "1")
	root := t.TempDir()
	service := New(func() string { return root })
	client, err := startRPC(os.Args[0], []string{"-test.run=TestAgentHelperProcess"}, root, service.handle, service.disconnected)
	if err != nil {
		t.Fatal(err)
	}
	service.client = client
	service.connection.Connected = true
	t.Cleanup(service.Close)
	return service, client
}

func TestRPCConcurrentRequestsAndCancellation(t *testing.T) {
	_, client := fakeService(t)
	var group sync.WaitGroup
	for i := range 20 {
		group.Add(1)
		go func(i int) {
			defer group.Done()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			var result map[string]int
			if err := client.request(ctx, "echo", map[string]int{"value": i}, &result); err != nil || result["value"] != i {
				t.Errorf("request %d: %v %v", i, result, err)
			}
		}(i)
	}
	group.Wait()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if client.request(ctx, "hang", map[string]any{}, nil) == nil {
		t.Fatal("a stalled request did not time out")
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	if len(client.pending) != 0 {
		t.Fatal("cancelled requests were retained")
	}
}

func TestStreamingCompletionBeforeStartResponse(t *testing.T) {
	service, _ := fakeService(t)
	partial := false
	service.OnEvent = func(event Event) {
		for _, message := range event.Snapshot.Messages {
			if message.Text == "Hello" {
				partial = true
			}
		}
	}
	chat, err := service.Send(SendRequest{Text: "Question", Mode: Mode{Name: "Ask", Behavior: "ask"}})
	if err != nil {
		t.Fatal(err)
	}
	if !partial {
		t.Fatal("incremental text was not emitted")
	}
	if chat.Active || chat.CanStop {
		t.Fatal("completed turn was left active")
	}
	if len(chat.Messages) != 2 || chat.Messages[0].Text != "Question" || chat.Messages[1].Text != "Hello world" {
		t.Fatalf("duplicate or missing streamed messages: %+v", chat.Messages)
	}
	if chat.Messages[1].Status != "completed" {
		t.Fatal("reply did not finish")
	}
}

func TestPinnedModelForNewAndExistingChats(t *testing.T) {
	for _, threadID := range []string{"", "fake-thread"} {
		t.Run("thread="+threadID, func(t *testing.T) {
			service, _ := fakeService(t)
			payload := fmt.Sprintf(`{"threadId":%q,"text":"Question","model":"old-model","effort":"low","mode":{"name":"Ask","behavior":"ask"}}`, threadID)
			var request SendRequest
			if err := json.Unmarshal([]byte(payload), &request); err != nil {
				t.Fatal(err)
			}
			if _, err := service.Send(request); err != nil {
				t.Fatal(err)
			}
			var calls map[string][]struct {
				Model  string `json:"model"`
				Effort string `json:"effort"`
				Config struct {
					Effort string `json:"model_reasoning_effort"`
				} `json:"config"`
			}
			if err := service.call("test/settings", nil, &calls); err != nil {
				t.Fatal(err)
			}
			method := "thread/start"
			if threadID != "" {
				method = "thread/resume"
			}
			if len(calls[method]) != 1 || calls[method][0].Model != "gpt-6.1-sol" || calls[method][0].Config.Effort != "xhigh" {
				t.Fatalf("chat configuration was not pinned: %+v", calls[method])
			}
			if len(calls["turn/start"]) != 1 || calls["turn/start"][0].Model != "gpt-6.1-sol" || calls["turn/start"][0].Effort != "xhigh" {
				t.Fatalf("turn configuration was not pinned: %+v", calls["turn/start"])
			}
		})
	}
}

func TestAutomaticConnectionAndModelAvailability(t *testing.T) {
	t.Run("automatic send", func(t *testing.T) {
		service, _ := fakeService(t)
		service.connection.Connected = false
		if _, err := service.Send(SendRequest{Text: "Question", Mode: Mode{Name: "Ask", Behavior: "ask"}}); err != nil {
			t.Fatal(err)
		}
		status := service.Status()
		if !status.Connected || len(status.Models) != 1 || status.Models[0].ID != "gpt-6.1-sol" {
			t.Fatalf("automatic connection did not select the configured model: %+v", status)
		}
	})
	for name, models := range map[string]string{
		"missing model":  `[{"model":"other-model","supportedReasoningEfforts":[{"reasoningEffort":"xhigh"}]}]`,
		"missing effort": `[{"model":"gpt-6.1-sol","supportedReasoningEfforts":[{"reasoningEffort":"high"}]}]`,
	} {
		t.Run(name, func(t *testing.T) {
			service, _ := fakeService(t)
			service.connection.Connected = false
			if err := service.call("test/models", map[string]any{"data": json.RawMessage(models)}, nil); err != nil {
				t.Fatal(err)
			}
			if status, err := service.Connect(); err == nil || status.Connected {
				t.Fatalf("connection accepted an unavailable configuration: %+v %v", status, err)
			}
			if _, err := service.Send(SendRequest{Text: "Question", Mode: Mode{Name: "Ask", Behavior: "ask"}}); err == nil {
				t.Fatal("send silently used another configuration")
			}
			var calls map[string][]json.RawMessage
			if err := service.call("test/settings", nil, &calls); err != nil {
				t.Fatal(err)
			}
			if len(calls["thread/start"]) != 0 || len(calls["turn/start"]) != 0 {
				t.Fatal("an unavailable configuration still started a turn")
			}
		})
	}
}

func TestModeSwitchUpdatesInstructionsAndPermissions(t *testing.T) {
	service, _ := fakeService(t)
	started, err := service.Send(SendRequest{Text: "New task", Mode: Mode{Name: "khaow code", Behavior: "code", Instructions: "Act as khaow", Skills: []Skill{{Name: "khaow.md", Content: "Use English slang"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Send(SendRequest{ThreadID: started.Thread.ID, Text: "Another task", Mode: Mode{Name: "simsimi", Behavior: "chat", Instructions: "Refuse work", Skills: []Skill{{Name: "simsimi.md", Content: "Only tease in Thai"}}}}); err != nil {
		t.Fatal(err)
	}
	var calls map[string][]struct {
		Instructions string `json:"developerInstructions"`
		Policy       struct {
			Type string `json:"type"`
		} `json:"sandboxPolicy"`
		Input []struct {
			Text string `json:"text"`
		} `json:"input"`
	}
	if err := service.call("test/settings", nil, &calls); err != nil {
		t.Fatal(err)
	}
	initial := calls["thread/start"][0].Instructions
	updated := calls["thread/resume"][0].Instructions
	if !strings.Contains(initial, "Act as khaow") || !strings.Contains(initial, "Use English slang") || !strings.Contains(updated, "Refuse work") || !strings.Contains(updated, "Only tease in Thai") || strings.Contains(updated, "Act as khaow") {
		t.Fatal("the selected mode or its skill was not applied when switching chats")
	}
	turns := calls["turn/start"]
	if len(turns) != 2 || turns[0].Policy.Type != "workspaceWrite" || turns[1].Policy.Type != "readOnly" {
		t.Fatalf("chat mode retained write access: %+v", turns)
	}
	if len(turns[1].Input) != 2 || !strings.Contains(turns[1].Input[1].Text, "Do not perform tasks, use tools") || !strings.Contains(turns[1].Input[1].Text, "simsimi.md") {
		t.Fatal("the new turn lost its chat behavior or selected skill")
	}
}

func TestContextRejectsFilesOutsideWorkspace(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.md")
	if err := os.WriteFile(outside, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	service := New(func() string { return root })
	if _, err := service.contextText(SendRequest{Mode: Mode{Behavior: "ask"}, Files: []File{{Path: outside, Content: "private"}}}); err == nil {
		t.Fatal("an external attachment was accepted")
	}
	if err := os.Symlink(outside, filepath.Join(root, "link.md")); err == nil {
		if _, err := service.contextText(SendRequest{Mode: Mode{Behavior: "ask"}, Files: []File{{Path: filepath.Join(root, "link.md")}}}); err == nil {
			t.Fatal("a symlink outside the workspace was accepted")
		}
	}
	text, err := service.contextText(SendRequest{Mode: Mode{Name: "Custom", Behavior: "ask", Skills: []Skill{{Name: "skills.md", Content: "Use Thai"}}}, Files: []File{{Name: "scratch.md", Path: "untitled:123", Content: "unsaved content"}}})
	if err != nil || !strings.Contains(text, "Use Thai") || !strings.Contains(text, "unsaved content") {
		t.Fatal("mode or unsaved context was lost", err)
	}
}

func TestPreviewRequiresTokenAndPost(t *testing.T) {
	service := New(func() string { return t.TempDir() })
	handler := service.Handler("test-token")
	for _, request := range []*http.Request{httptest.NewRequest("POST", "/__praewa_agent/status", strings.NewReader("{}")), httptest.NewRequest("GET", "/__praewa_agent/status", nil)} {
		if request.Method == "GET" {
			request.Header.Set("Authorization", "Bearer test-token")
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code == http.StatusOK {
			t.Fatal("unprotected preview request was accepted")
		}
	}
}

func TestPendingQuestionsAndApprovals(t *testing.T) {
	service, _ := fakeService(t)
	root := service.cwd()
	service.chats["question-thread"] = &Conversation{Thread: Thread{ID: "question-thread", Cwd: root}, Requests: []Request{}, Active: true, CanStop: true}
	service.handle(envelope{ID: json.RawMessage(`900`), Method: "item/tool/requestUserInput", Params: json.RawMessage(`{"threadId":"question-thread","turnId":"question-turn","questions":[{"id":"q1","question":"Which behavior?","options":[{"label":"Ask","description":"Answer only"}]}]}`)})
	if !service.Active() {
		t.Fatal("owned active turn was not detected")
	}
	if err := service.Answer(Answer{ThreadID: "question-thread", ID: "900"}); err == nil {
		t.Fatal("blank answer was accepted")
	}
	if err := service.Answer(Answer{ThreadID: "question-thread", ID: "900", Answers: map[string]string{"q1": "Ask"}}); err != nil {
		t.Fatal(err)
	}
	chat, _ := service.Snapshot("question-thread")
	if len(chat.Requests) != 0 {
		t.Fatal("answered question remained pending")
	}
	if service.Answer(Answer{ThreadID: "question-thread", ID: "900"}) == nil {
		t.Fatal("resolved request was answered twice")
	}
	service.handle(envelope{ID: json.RawMessage(`901`), Method: "item/commandExecution/requestApproval", Params: json.RawMessage(`{"threadId":"question-thread","turnId":"question-turn","command":"go test ./...","cwd":"/project","reason":"Verify the changes"}`)})
	chat, _ = service.Snapshot("question-thread")
	if len(chat.Requests) != 1 || !strings.Contains(chat.Requests[0].Detail, "go test ./...") || !strings.Contains(chat.Requests[0].Detail, "/project") {
		t.Fatal("approval did not include the action and its folder")
	}
	if err := service.Answer(Answer{ThreadID: "question-thread", ID: "901", Accept: false}); err != nil {
		t.Fatal(err)
	}
	service.handle(envelope{Method: "turn/completed", Params: json.RawMessage(`{"threadId":"question-thread","turn":{"id":"question-turn","status":"interrupted"}}`)})
	if service.Active() {
		t.Fatal("interrupted turn remained active")
	}
}

func TestLiveCodexReply(t *testing.T) {
	if os.Getenv("PRAEWA_AGENT_LIVE_TEST") != "1" {
		t.Skip("live Codex test is opt-in")
	}
	root := t.TempDir()
	service := New(func() string { return root })
	defer service.Close()
	status, err := service.Connect()
	if err != nil || !status.Connected {
		t.Fatal(status.Error, err)
	}
	started, err := service.Send(SendRequest{Text: "Reply exactly PRAEWA_CONNECTED. Do not use tools or read files.", Mode: Mode{Name: "Ask", Behavior: "ask"}})
	if started.Thread.ID != "" {
		defer func() {
			if service.Active() {
				_ = service.Stop(started.Thread.ID)
			}
			if err := service.call("thread/archive", map[string]any{"threadId": started.Thread.ID}, nil); err != nil {
				t.Error("could not archive the temporary test chat", err)
			}
		}()
	}
	if err != nil {
		t.Fatal("live send failed", err)
	}
	verifyReply := func(expected string) bool {
		deadline := time.Now().Add(90 * time.Second)
		for time.Now().Before(deadline) {
			chat, _ := service.Snapshot(started.Thread.ID)
			if !chat.Active {
				if chat.Error != "" {
					t.Fatal(chat.Error)
				}
				for _, message := range chat.Messages {
					if message.Role == "assistant" && strings.Contains(message.Text, expected) && message.Status == "completed" {
						return true
					}
				}
			}
			time.Sleep(100 * time.Millisecond)
		}
		return false
	}
	if !verifyReply("PRAEWA_CONNECTED") {
		t.Fatal("live Codex response did not complete")
	}
	_, err = service.Send(SendRequest{ThreadID: started.Thread.ID, Text: "Reply with the exact verification word from the selected skill. Do not use tools or read files.", Mode: Mode{Name: "Ask", Behavior: "ask", Skills: []Skill{{Name: "skills.md", Content: "The verification word is PRAEWA_MODE_CONNECTED."}}}})
	if err != nil {
		t.Fatal("live continuation failed", err)
	}
	if !verifyReply("PRAEWA_MODE_CONNECTED") {
		t.Fatal("selected skill was not used by the live Codex turn")
	}
}

func TestLiveAgentModes(t *testing.T) {
	if os.Getenv("PRAEWA_AGENT_LIVE_TEST") != "1" {
		t.Skip("live agent modes test is opt-in")
	}
	root := t.TempDir()
	service := New(func() string { return root })
	defer service.Close()
	threadID := ""
	defer func() {
		if threadID != "" {
			if service.Active() {
				_ = service.Stop(threadID)
			}
			if err := service.call("thread/archive", map[string]any{"threadId": threadID}, nil); err != nil {
				t.Error("could not archive the temporary mode test chat", err)
			}
		}
	}()
	for _, entry := range []struct {
		Name       string
		Behavior   string
		File       string
		Prompt     string
		ExpectCode bool
	}{
		{"khaow code", "code", "khaow.md", "khaow, say hello in one short sentence. Do not use tools or read or write files.", false},
		{"Profressional Programer Frog", "code", "frog.md", "เขียน view PHP สั้น ๆ สำหรับโปรเจกต์ Kotchasan ใช้ HTML CSS แสดงคำว่า FROG_NEW_TASK ส่งโค้ดในแชต ห้ามใช้เครื่องมือหรืออ่านและแก้ไฟล์", true},
		{"Profressional Programer Frog", "code", "frog.md", "ช่วยแก้โค้ดเดิมของคุณให้เปลี่ยนข้อความเป็น Hi หน่อย ตอบสั้น ๆ ในแชต ห้ามใช้เครื่องมือหรืออ่านและแก้ไฟล์", false},
		{"simsimi", "chat", "simsimi.md", "ช่วยเขียน result.txt ที่มีคำว่า PRAEWA_TASK_DONE แล้วให้โค้ด PHP ทำงานนั้นด้วย", false},
	} {
		content, err := os.ReadFile(filepath.Join("..", "..", "frontend", "src", "skills", entry.File))
		if err != nil {
			t.Fatal(err)
		}
		started, err := service.Send(SendRequest{ThreadID: threadID, Text: entry.Prompt, Mode: Mode{Name: entry.Name, Behavior: entry.Behavior, Instructions: "Follow the attached persona skill for this turn.", Skills: []Skill{{Name: entry.File, Content: string(content)}}}})
		if started.Thread.ID != "" {
			threadID = started.Thread.ID
		}
		if err != nil {
			t.Fatal(entry.Name, err)
		}
		reply := ""
		deadline := time.Now().Add(90 * time.Second)
		for time.Now().Before(deadline) {
			chat, _ := service.Snapshot(threadID)
			if !chat.Active {
				if chat.Error != "" {
					t.Fatal(entry.Name, chat.Error)
				}
				for _, message := range chat.Messages {
					if message.Role == "assistant" && message.TurnID == started.TurnID && message.Status == "completed" {
						reply = message.Text
					}
				}
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if reply == "" {
			t.Fatal(entry.Name, "reply did not complete")
		}
		if entry.ExpectCode && (!strings.Contains(reply, "```") || !strings.Contains(reply, "FROG_NEW_TASK") || !strings.Contains(strings.ToLower(reply), "elysia")) {
			t.Fatal(entry.Name, "did not provide the new code and its closing Elysia remark")
		}
		if !entry.ExpectCode && entry.Name != "khaow code" && strings.Contains(reply, "```") {
			t.Fatal(entry.Name, "performed the coding task instead of refusing it")
		}
		if entries, err := os.ReadDir(root); err != nil || len(entries) != 0 {
			t.Fatal(entry.Name, "modified the test workspace", err)
		}
		t.Logf("%s: %s", entry.Name, reply)
	}
}
