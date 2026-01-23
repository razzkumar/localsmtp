package sse

import "sync"

type Hub struct {
	mu   sync.RWMutex
	subs map[string]map[chan []byte]struct{}
}

func NewHub() *Hub {
	return &Hub{subs: make(map[string]map[chan []byte]struct{})}
}

func (h *Hub) Subscribe(email string) (chan []byte, func()) {
	ch := make(chan []byte, 8)
	h.mu.Lock()
	if _, ok := h.subs[email]; !ok {
		h.subs[email] = make(map[chan []byte]struct{})
	}
	h.subs[email][ch] = struct{}{}
	h.mu.Unlock()

	return ch, func() {
		h.mu.Lock()
		if subscribers, ok := h.subs[email]; ok {
			delete(subscribers, ch)
			if len(subscribers) == 0 {
				delete(h.subs, email)
			}
		}
		h.mu.Unlock()
		close(ch)
	}
}

func (h *Hub) Broadcast(emails []string, payload []byte) {
	if len(emails) == 0 {
		return
	}
	unique := map[string]struct{}{}
	for _, email := range emails {
		if email == "" {
			continue
		}
		unique[email] = struct{}{}
	}

	h.mu.RLock()
	defer h.mu.RUnlock()
	for email := range unique {
		for ch := range h.subs[email] {
			select {
			case ch <- payload:
			default:
			}
		}
	}
}
