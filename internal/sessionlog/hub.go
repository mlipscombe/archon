package sessionlog

import (
	"sync"
	"time"
)

const maxBufferedEntries = 500

type Entry struct {
	Timestamp time.Time `json:"timestamp"`
	Stream    string    `json:"stream"`
	Message   string    `json:"message"`
}

type Hub struct {
	mu       sync.RWMutex
	sessions map[string]*sessionBuffer
}

type sessionBuffer struct {
	entries     []Entry
	subscribers map[chan Entry]struct{}
}

func NewHub() *Hub {
	return &Hub{sessions: map[string]*sessionBuffer{}}
}

func (h *Hub) Publish(sessionID, stream, message string) {
	h.mu.Lock()
	buffer := h.ensureSessionLocked(sessionID)
	entry := Entry{Timestamp: time.Now().UTC(), Stream: stream, Message: message}
	buffer.entries = append(buffer.entries, entry)
	if len(buffer.entries) > maxBufferedEntries {
		buffer.entries = append([]Entry(nil), buffer.entries[len(buffer.entries)-maxBufferedEntries:]...)
	}
	subscribers := make([]chan Entry, 0, len(buffer.subscribers))
	for subscriber := range buffer.subscribers {
		subscribers = append(subscribers, subscriber)
	}
	h.mu.Unlock()

	for _, subscriber := range subscribers {
		select {
		case subscriber <- entry:
		default:
		}
	}
}

func (h *Hub) Subscribe(sessionID string) ([]Entry, chan Entry, func()) {
	h.mu.Lock()
	buffer := h.ensureSessionLocked(sessionID)
	snapshot := append([]Entry(nil), buffer.entries...)
	ch := make(chan Entry, 64)
	buffer.subscribers[ch] = struct{}{}
	h.mu.Unlock()

	unsubscribe := func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		buffer, ok := h.sessions[sessionID]
		if !ok {
			close(ch)
			return
		}
		if _, ok := buffer.subscribers[ch]; ok {
			delete(buffer.subscribers, ch)
			close(ch)
		}
	}

	return snapshot, ch, unsubscribe
}

func (h *Hub) ensureSessionLocked(sessionID string) *sessionBuffer {
	buffer, ok := h.sessions[sessionID]
	if ok {
		return buffer
	}
	buffer = &sessionBuffer{subscribers: map[chan Entry]struct{}{}}
	h.sessions[sessionID] = buffer
	return buffer
}
