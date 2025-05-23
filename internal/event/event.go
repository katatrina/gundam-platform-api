package event

// Event đại diện cho một sự kiện trong hệ thống
type Event struct {
	Topic string      // Ví dụ: "auction:123", "user:abc"
	Type  string      // Loại sự kiện: new_participant, new_bid, auction_ended
	Data  interface{} // Dữ liệu sự kiện (tùy thuộc loại)
}

const (
	EventTypeNewParticipant = "new_participant" // Người dùng mới tham gia
	EventTypeNewBid         = "new_bid"         // Người dùng mới đặt giá
	EventTypeAuctionEnded   = "auction_ended"   // Phiên đấu giá đã kết thúc
)

// EventSender là interface cho đại diện cho server gửi sự kiện đến client
type EventSender interface {
	Register(topic string, client chan Event)
	Unregister(topic string, client chan Event)
	Broadcast(event Event)
	Run()
}
