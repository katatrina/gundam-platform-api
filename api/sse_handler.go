package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/katatrina/gundam-BE/internal/event"
)

// @Summary		Stream auction events via Server-Sent Events
// @Description	Establishes an SSE connection to receive real-time updates about an auction
// @Tags			auctions
// @Produce		text/event-stream
// @Param			auctionID	path		string	true	"Auction ID"
// @Success		200			{string}	string	"Event stream. Data will be sent as SSE events with format: 'event: {eventType}\ndata: {jsonData}'"
// @Failure		400			{object}	object	"Invalid auction ID format"
// @Router			/v1/auctions/{auctionID}/stream [get]
func (server *Server) streamAuctionEvents(c *gin.Context) {
	auctionID := c.Param("auctionID")
	if _, err := uuid.Parse(auctionID); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(fmt.Errorf("invalid auction ID format")))
		return
	}
	
	// TODO: Có thể kiểm tra thêm:
	// - Kiểm tra xem phiên đấu giá có tồn tại không
	// - Kiểm tra xem người dùng có quyền truy cập vào phiên đấu giá này không
	// - Phiên đấu giá có đang diễn ra không
	
	topic := fmt.Sprintf("auction:%s", auctionID)
	
	// Thiết lập header SSE
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Status(http.StatusOK)
	
	// Tạo channel cho client
	clientChan := make(chan event.Event)
	server.eventSender.Register(topic, clientChan)
	defer server.eventSender.Unregister(topic, clientChan)
	
	// Gửi sự kiện tới client
	for {
		select {
		case event := <-clientChan:
			data, _ := json.Marshal(event.Data)
			fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event.Type, data)
			c.Writer.Flush()
		case <-c.Request.Context().Done():
			return
		}
	}
}
