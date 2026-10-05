package main

import "praewa-code/internal/agent"

type AgentSendResult struct {
	Conversation agent.Conversation `json:"conversation"`
	Error        string             `json:"error"`
}

func (a *App) AgentConnect() agent.Connection {
	status, _ := a.agent.Connect()
	return status
}

func (a *App) AgentStatus() agent.Connection                        { return a.agent.Status() }
func (a *App) AgentThreads(cursor string) (agent.ThreadPage, error) { return a.agent.Threads(cursor) }
func (a *App) AgentLoad(id string) (agent.Conversation, error)      { return a.agent.Load(id) }
func (a *App) AgentSnapshot(id string) (agent.Conversation, error)  { return a.agent.Snapshot(id) }
func (a *App) AgentSend(request agent.SendRequest) AgentSendResult {
	conversation, err := a.agent.Send(request)
	result := AgentSendResult{Conversation: conversation}
	if err != nil {
		result.Error = err.Error()
	}
	return result
}
func (a *App) AgentStop(id string) error             { return a.agent.Stop(id) }
func (a *App) AgentAnswer(answer agent.Answer) error { return a.agent.Answer(answer) }
