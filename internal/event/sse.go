package event

import (
	"sync"
	
	"github.com/rs/zerolog/log"
)

type SSEServer struct {
	clients map[string]map[chan Event]bool
	events  chan Event
	mu      sync.Mutex
}

func NewSSEServer() EventSender {
	return &SSEServer{
		clients: make(map[string]map[chan Event]bool),
		events:  make(chan Event),
	}
}

// Register đăng ký client vào topic.
func (s *SSEServer) Register(topic string, client chan Event) {
	s.mu.Lock()
	if _, ok := s.clients[topic]; !ok {
		s.clients[topic] = make(map[chan Event]bool)
	}
	s.clients[topic][client] = true
	s.mu.Unlock()
	log.Info().Msgf("New client registered to topic %s. Total clients: %d", topic, len(s.clients[topic]))
}

// Unregister hủy đăng ký client khỏi topic.
func (s *SSEServer) Unregister(topic string, client chan Event) {
	s.mu.Lock()
	if clients, ok := s.clients[topic]; ok {
		delete(clients, client)
		close(client)
		if len(clients) == 0 {
			delete(s.clients, topic)
		}
	}
	s.mu.Unlock()
	log.Info().Msgf("Client unregistered from topic %s. Remaining clients: %d", topic, len(s.clients[topic]))
}

// Broadcast gửi sự kiện tới tất cả client của topic
func (s *SSEServer) Broadcast(event Event) {
	s.events <- event
}

// Run xử lý luồng sự kiện
func (s *SSEServer) Run() {
	for event := range s.events {
		s.mu.Lock()
		clients := s.clients[event.Topic]
		s.mu.Unlock()
		
		var wg sync.WaitGroup
		for client := range clients {
			wg.Add(1)
			go func(c chan Event) {
				defer wg.Done()
				// Send to client with timeout
			}(client)
		}
		wg.Wait()
	}
}
