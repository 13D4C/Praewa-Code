package agent

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

func (s *Service) Handler(token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != http.MethodPost {
			http.Error(w, "Use POST", http.StatusMethodNotAllowed)
			return
		}
		var request struct {
			ID     string      `json:"id"`
			Cursor string      `json:"cursor"`
			Send   SendRequest `json:"send"`
			Answer Answer      `json:"answer"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2*1024*1024)).Decode(&request); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}
		var result any
		var err error
		switch strings.TrimPrefix(r.URL.Path, "/__praewa_agent/") {
		case "connect":
			result, _ = s.Connect()
		case "status":
			result = s.Status()
		case "threads":
			result, err = s.Threads(request.Cursor)
		case "load":
			result, err = s.Load(request.ID)
		case "snapshot":
			result, err = s.Snapshot(request.ID)
		case "send":
			conversation, sendErr := s.Send(request.Send)
			message := ""
			if sendErr != nil {
				message = sendErr.Error()
			}
			result = map[string]any{"conversation": conversation, "error": message}
		case "stop":
			err = s.Stop(request.ID)
		case "answer":
			err = s.Answer(request.Answer)
		default:
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		if err := json.NewEncoder(w).Encode(result); err != nil {
			fmt.Fprintln(w, `{"error":"Response could not be encoded"}`)
		}
	})
}
