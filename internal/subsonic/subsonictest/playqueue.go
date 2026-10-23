package subsonictest

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// PlayQueue is a play queue as the server holds it: what savePlayQueue
// stored for a user.
type PlayQueue struct {
	IDs       []string
	Current   string
	Position  time.Duration
	Changed   time.Time
	ChangedBy string
}

// PlayQueueFault is how getPlayQueue and savePlayQueue misbehave.
type PlayQueueFault int

// The faults. The play queue calls are the only ones affected.
const (
	// PlayQueueWorks is a server that keeps play queues.
	PlayQueueWorks PlayQueueFault = iota
	// PlayQueueMissing answers HTTP 404, as a server without the calls does.
	PlayQueueMissing
	// PlayQueueBroken answers HTTP 500.
	PlayQueueBroken
	// PlayQueueHangs never answers, until the client gives up.
	PlayQueueHangs
)

// SetPlayQueueFault makes the play queue calls fail as f says.
func (s *Server) SetPlayQueueFault(f PlayQueueFault) {
	s.mu.Lock()
	s.fault = f
	s.mu.Unlock()
}

// PlayQueue is the play queue stored for user, and whether there is one.
func (s *Server) PlayQueue(user string) (PlayQueue, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	q, ok := s.queues[user]
	return q, ok
}

// SetPlayQueue stores a play queue for user, as another device's
// savePlayQueue would have. An empty ID list clears it.
func (s *Server) SetPlayQueue(user string, q PlayQueue) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(q.IDs) == 0 {
		delete(s.queues, user)
		return
	}
	s.queues[user] = q
}

// servePlayQueue answers getPlayQueue and savePlayQueue from the stored
// queue of the authorized user, which is not a recorded fixture because a
// fixture can't change with what was saved. Its entries carry only IDs.
func (s *Server) servePlayQueue(
	w http.ResponseWriter, r *http.Request, endpoint string, params url.Values,
) {
	s.mu.Lock()
	fault := s.fault
	s.mu.Unlock()
	switch fault {
	case PlayQueueMissing:
		http.NotFound(w, r)
		return
	case PlayQueueBroken:
		http.Error(w, "broken", http.StatusInternalServerError)
		return
	case PlayQueueHangs:
		<-r.Context().Done()
		return
	}
	user := params.Get("u")
	resp := map[string]any{
		"status":        "ok",
		"version":       "1.16.1",
		"type":          "navidrome",
		"serverVersion": "0.64.2 (10114574)",
		"openSubsonic":  true,
	}
	if endpoint == "savePlayQueue" {
		q := PlayQueue{
			IDs:       params["id"],
			Current:   params.Get("current"),
			Changed:   time.Now().UTC(),
			ChangedBy: params.Get("c"),
		}
		if ms, err := strconv.ParseInt(params.Get("position"), 10, 64); err == nil {
			q.Position = time.Duration(ms) * time.Millisecond
		}
		s.SetPlayQueue(user, q)
	} else if q, ok := s.PlayQueue(user); ok {
		entries := make([]map[string]any, len(q.IDs))
		for i, id := range q.IDs {
			entries[i] = map[string]any{"id": id}
		}
		resp["playQueue"] = map[string]any{
			"entry":     entries,
			"current":   q.Current,
			"position":  q.Position.Milliseconds(),
			"username":  user,
			"changed":   q.Changed.Format(time.RFC3339Nano),
			"changedBy": q.ChangedBy,
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"subsonic-response": resp})
}
