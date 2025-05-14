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
		if clients, ok := s.clients[event.Topic]; ok {
			for client := range clients {
				select {
				case client <- event:
				default:
					// Bỏ qua nếu client không nhận được
				}
			}
		}
		s.mu.Unlock()
	}
}
