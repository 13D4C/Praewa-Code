package agent

import "encoding/json"

type Model struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Default     bool   `json:"default"`
}

type Connection struct {
	Connected bool    `json:"connected"`
	Account   string  `json:"account"`
	Transport string  `json:"transport"`
	Error     string  `json:"error"`
	Models    []Model `json:"models"`
}

type Thread struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	Preview   string       `json:"preview"`
	Cwd       string       `json:"cwd"`
	Model     string       `json:"model"`
	UpdatedAt int64        `json:"updatedAt"`
	Status    ThreadStatus `json:"status"`
	Turns     []turn       `json:"turns,omitempty"`
}

type ThreadStatus struct {
	Type string `json:"type"`
}

type TurnError struct {
	Message string `json:"message"`
}

type ThreadPage struct {
	Data       []Thread `json:"data"`
	NextCursor *string  `json:"nextCursor"`
}

type turn struct {
	ID        string     `json:"id"`
	Status    string     `json:"status"`
	StartedAt int64      `json:"startedAt"`
	Items     []item     `json:"items"`
	Error     *TurnError `json:"error"`
}

type item struct {
	ID       string        `json:"id"`
	Type     string        `json:"type"`
	Text     string        `json:"text"`
	Phase    string        `json:"phase"`
	ClientID string        `json:"clientId"`
	Command  string        `json:"command"`
	Status   string        `json:"status"`
	Content  []TextContent `json:"content"`
}

type TextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type Message struct {
	ID        string `json:"id"`
	TurnID    string `json:"turnId"`
	Role      string `json:"role"`
	Text      string `json:"text"`
	Phase     string `json:"phase"`
	Status    string `json:"status"`
	CreatedAt int64  `json:"createdAt"`
}

type Question struct {
	ID       string           `json:"id"`
	Question string           `json:"question"`
	Options  []QuestionOption `json:"options"`
}

type QuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

type Request struct {
	ID        string          `json:"id"`
	ThreadID  string          `json:"threadId"`
	TurnID    string          `json:"turnId"`
	Kind      string          `json:"kind"`
	Title     string          `json:"title"`
	Detail    string          `json:"detail"`
	Questions []Question      `json:"questions"`
	Method    string          `json:"-"`
	RPCID     json.RawMessage `json:"-"`
	Params    json.RawMessage `json:"-"`
}

type Conversation struct {
	Thread   Thread    `json:"thread"`
	Sequence int64     `json:"sequence"`
	Messages []Message `json:"messages"`
	Requests []Request `json:"requests"`
	TurnID   string    `json:"turnId"`
	Active   bool      `json:"active"`
	CanStop  bool      `json:"canStop"`
	Error    string    `json:"error"`
}

type Event struct {
	ThreadID string        `json:"threadId"`
	Snapshot *Conversation `json:"snapshot,omitempty"`
}

type Skill struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

type Mode struct {
	Name         string  `json:"name"`
	Behavior     string  `json:"behavior"`
	Instructions string  `json:"instructions"`
	Skills       []Skill `json:"skills"`
}

type File struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Content string `json:"content"`
}

type SendRequest struct {
	ID       string `json:"id"`
	ThreadID string `json:"threadId"`
	Text     string `json:"text"`
	Mode     Mode   `json:"mode"`
	Files    []File `json:"files"`
}

type Answer struct {
	ThreadID string            `json:"threadId"`
	ID       string            `json:"id"`
	Accept   bool              `json:"accept"`
	Answers  map[string]string `json:"answers"`
}
