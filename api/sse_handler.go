package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
	
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/event"
	"github.com/rs/zerolog/log"
)

//	@Summary		Stream auction events via Server-Sent Events
//	@Description	Establishes an SSE connection to receive real-time updates about an auction
//	@Tags			auctions
//	@Produce		text/event-stream
//	@Param			auctionID	path	string	true	"Auction ID"
//	@Success		200			"SSE connection established"
//	@Failure		400			"Invalid auction ID format"
//	@Failure		404			"Auction not found"
//	@Failure		500			"Internal server error"
//	@Router			/auctions/{auctionID}/stream [get]
func (server *Server) streamAuctionEvents(c *gin.Context) {
	auctionIDStr := c.Param("auctionID")
	auctionID, err := uuid.Parse(auctionIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(fmt.Errorf("invalid auction ID format")))
		return
	}
	
	// Kiểm tra xem phiên đấu giá có tồn tại không
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	_, err = server.dbStore.GetAuctionByID(ctx, auctionID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("auction with ID %s not found", auctionIDStr)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		err = fmt.Errorf("failed to get auction: %w", err)
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// TODO: Có thể kiểm tra thêm:
	// - Kiểm tra xem người dùng có quyền truy cập vào phiên đấu giá này không
	// - Phiên đấu giá có đang diễn ra không
	
	topic := fmt.Sprintf("auction:%s", auctionIDStr)
	
	// Thiết lập header SSE
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	
	c.Status(http.StatusOK)
	
	// Log new connection
	log.Info().Str("auction_id", auctionIDStr).Msg("New SSE connection established")
	
	// Tạo channel cho client
	clientChan := make(chan event.Event, 10) // Buffer để tránh blocking
	
	server.eventSender.Register(topic, clientChan)
	defer func() {
		server.eventSender.Unregister(topic, clientChan)
		close(clientChan) // Close channel sau khi unregister
	}()
	
	// Gửi event kết nối thành công
	connectionData := map[string]interface{}{
		"auction_id": auctionIDStr,                    // ID của phiên đấu giá
		"timestamp":  time.Now().Format(time.RFC3339), // Thời gian kết nối
		"status":     "connected",                     // Trạng thái kết nối
	}
	if data, err := json.Marshal(connectionData); err == nil {
		fmt.Fprintf(c.Writer, "event: connected\ndata: %s\n\n", data)
		c.Writer.Flush()
	}
	
	// Heartbeat ticker
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	// Gửi sự kiện tới client
	for {
		select {
		case event := <-clientChan:
			// Kiểm tra nil data
			if event.Data == nil {
				log.Warn().Msg("Received event with nil data, skipping")
				continue
			}
			
			data, err := json.Marshal(event.Data)
			if err != nil {
				// Log error và gửi error event
				log.Error().Err(err).Msgf("failed to serialize event data: %v, event: %+v", err, event)
				errorData := map[string]string{
					"error":      "failed to serialize event data", // Mô tả lỗi
					"event_type": event.Type,                       // Loại sự kiện
					"timestamp":  time.Now().Format(time.RFC3339),  // Thời gian xảy ra lỗi
				}
				if errorJson, jsonErr := json.Marshal(errorData); jsonErr == nil {
					fmt.Fprintf(c.Writer, "event: error\ndata: %s\n\n", errorJson)
					c.Writer.Flush()
				}
				continue
			}
			
			fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event.Type, data)
			c.Writer.Flush()
		
		case <-ticker.C:
			// Gửi heartbeat để maintain connection
			fmt.Fprintf(c.Writer, ": heartbeat\n\n")
			c.Writer.Flush()
		
		case <-c.Request.Context().Done():
			// Client disconnected hoặc server shutdown
			log.Info().Str("auction_id", auctionIDStr).Msg("SSE connection closed")
			return
		}
	}
}
