package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/katatrina/gundam-BE/internal/event"
)

func (server *Server) streamAuctionEvents(c *gin.Context) {
	auctionID := c.Param("auctionID")
	if _, err := uuid.Parse(auctionID); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(fmt.Errorf("invalid auction ID format")))
		return
	}
	
	topic := fmt.Sprintf("auction:%s", auctionID)
	
	// Thiết lập header SSE
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	
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
