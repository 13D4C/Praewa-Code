package agent

import (
	"encoding/json"
	"errors"
	"strings"
)

func parseRequest(message envelope, threadID, turnID string) *Request {
	request := &Request{ID: string(message.ID), RPCID: message.ID, Method: message.Method, Params: message.Params, ThreadID: threadID, TurnID: turnID}
	var params struct {
		Reason      string          `json:"reason"`
		Command     string          `json:"command"`
		Cwd         string          `json:"cwd"`
		GrantRoot   string          `json:"grantRoot"`
		Questions   []Question      `json:"questions"`
		Permissions json.RawMessage `json:"permissions"`
	}
	_ = json.Unmarshal(message.Params, &params)
	switch message.Method {
	case "item/commandExecution/requestApproval", "item/fileChange/requestApproval":
		request.Kind = "approval"
		request.Title = "Allow this command?"
		if message.Method == "item/fileChange/requestApproval" {
			request.Title = "Allow these file changes?"
			params.Command = ""
			if params.GrantRoot != "" {
				params.Command = "Write access: " + params.GrantRoot
			}
		} else if params.Cwd != "" {
			params.Command += "\nFolder: " + params.Cwd
		}
		request.Detail = strings.TrimSpace(params.Reason + "\n" + params.Command)
	case "item/permissions/requestApproval":
		request.Kind = "approval"
		request.Title = "Allow the requested access?"
		request.Detail = strings.TrimSpace(params.Reason + "\n" + string(params.Permissions))
	case "item/tool/requestUserInput":
		request.Kind = "question"
		request.Title = "Agent needs your input"
		request.Questions = params.Questions
	default:
		return nil
	}
	return request
}

func (s *Service) Answer(answer Answer) error {
	s.mu.Lock()
	chat := s.chats[answer.ThreadID]
	var selected *Request
	if chat != nil {
		for _, request := range chat.Requests {
			if request.ID == answer.ID {
				copy := request
				selected = &copy
				break
			}
		}
	}
	client := s.client
	s.mu.Unlock()
	if selected == nil || client == nil {
		return errors.New("This request is no longer pending")
	}
	var result any
	switch selected.Method {
	case "item/commandExecution/requestApproval", "item/fileChange/requestApproval":
		decision := "decline"
		if answer.Accept {
			decision = "accept"
		}
		result = map[string]any{"decision": decision}
	case "item/permissions/requestApproval":
		permissions := json.RawMessage(`{}`)
		if answer.Accept {
			var params struct {
				Permissions json.RawMessage `json:"permissions"`
			}
			_ = json.Unmarshal(selected.Params, &params)
			if len(params.Permissions) != 0 {
				permissions = params.Permissions
			}
		}
		result = map[string]any{"permissions": permissions, "scope": "turn"}
	case "item/tool/requestUserInput":
		answers := make(map[string]any)
		for _, question := range selected.Questions {
			text := strings.TrimSpace(answer.Answers[question.ID])
			if text == "" {
				return errors.New("Answer each question before continuing")
			}
			answers[question.ID] = map[string]any{"answers": []string{text}}
		}
		result = map[string]any{"answers": answers}
	}
	if err := client.write(map[string]any{"id": selected.RPCID, "result": result}); err != nil {
		return err
	}
	s.mu.Lock()
	chat = s.chats[answer.ThreadID]
	if chat == nil {
		s.mu.Unlock()
		return nil
	}
	filtered := chat.Requests[:0]
	for _, request := range chat.Requests {
		if request.ID != answer.ID {
			filtered = append(filtered, request)
		}
	}
	chat.Requests = filtered
	chat.Sequence++
	snapshot := copyConversation(chat)
	s.mu.Unlock()
	s.emit(snapshot)
	return nil
}

func (s *Service) rejectUnsupported(message envelope) {
	s.mu.Lock()
	client := s.client
	s.mu.Unlock()
	if client != nil {
		_ = client.write(map[string]any{"id": message.ID, "error": rpcError{Code: -32601, Message: "This request is not supported by the Praewa Code chat panel"}})
	}
}
