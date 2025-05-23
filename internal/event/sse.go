package event

import (
	"sync"
	"time"
	
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
		events:  make(chan Event, 100), // Thêm buffer cho event channel
	}
}

// Register đăng ký client vào topic.
func (s *SSEServer) Register(topic string, client chan Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if _, ok := s.clients[topic]; !ok {
		s.clients[topic] = make(map[chan Event]bool)
	}
	s.clients[topic][client] = true
	log.Info().Msgf("New client registered to topic %s. Total clients: %d", topic, len(s.clients[topic]))
}

// Unregister hủy đăng ký client khỏi topic.
func (s *SSEServer) Unregister(topic string, client chan Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	
	if clients, ok := s.clients[topic]; ok {
		delete(clients, client)
		// Không close channel ở đây, sẽ close ở handler
		
		if len(clients) == 0 {
			delete(s.clients, topic)
		}
		log.Info().Msgf("Client unregistered from topic %s. Remaining clients: %d", topic, len(clients))
	}
}

// Broadcast gửi sự kiện tới tất cả client của topic
func (s *SSEServer) Broadcast(event Event) {
	select {
	case s.events <- event:
		// Event sent successfully
	case <-time.After(1 * time.Second):
		// Timeout - event channel có thể bị đầy
		log.Warn().Str("topic", event.Topic).Str("type", event.Type).Msg("Failed to broadcast event - timeout")
	}
}

// Run xử lý luồng sự kiện
func (s *SSEServer) Run() {
	for event := range s.events {
		s.mu.Lock()
		// Copy map để tránh giữ lock quá lâu
		clientsCopy := make([]chan Event, 0, len(s.clients[event.Topic]))
		for client := range s.clients[event.Topic] {
			clientsCopy = append(clientsCopy, client)
		}
		s.mu.Unlock()
		
		// Gửi event cho từng client
		var wg sync.WaitGroup
		for _, client := range clientsCopy {
			wg.Add(1)
			go func(c chan Event) {
				defer wg.Done()
				
				// Gửi với timeout để tránh block
				select {
				case c <- event:
					// Gửi thành công
				case <-time.After(2 * time.Second):
					// Client chậm hoặc bị block
					log.Warn().
						Str("topic", event.Topic).
						Str("type", event.Type).
						Msg("Client channel blocked, skipping event")
				}
			}(client)
		}
		
		// Đợi tất cả goroutines hoàn thành
		wg.Wait()
	}
}

// Stop dừng SSE server (optional - thêm để graceful shutdown)
// TODO: Triển khai graceful shutdown cho SSE server
func (s *SSEServer) Stop() {
	close(s.events)
	log.Info().Msg("SSE server stopped")
}
