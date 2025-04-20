package api

import (
	"errors"
	"fmt"
	"net/http"
	
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/token"
)

// CreateExchangeOfferRequest định nghĩa thông tin cần thiết để tạo đề xuất trao đổi 1-1
type CreateExchangeOfferRequest struct {
	PostID             string  `json:"post_id" binding:"required,uuid"`
	PosterGundamID     int64   `json:"poster_gundam_id" binding:"required"`  // ID Gundam của người đăng bài
	OffererGundamID    int64   `json:"offerer_gundam_id" binding:"required"` // ID Gundam của người đề xuất
	PayerID            *string `json:"payer_id"`                             // ID người bù tiền (poster_id hoặc offerer_id, hoặc null)
	CompensationAmount *int64  `json:"compensation_amount"`                  // Số tiền bù (null nếu không có bù tiền)
}

// ExchangeOfferResponse định nghĩa thông tin trả về sau khi tạo đề xuất
type ExchangeOfferResponse struct {
	db.ExchangeOffer
	OfferItems []db.GundamDetails `json:"offer_items"` // Danh sách chi tiết Gundam trong đề xuất
}

func (server *Server) createExchangeOffer(c *gin.Context) {
	var req CreateExchangeOfferRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	// Lấy thông tin người dùng đã đăng nhập
	authPayload := c.MustGet(authorizationPayloadKey).(*token.Payload)
	offererID := authPayload.Subject
	
	postID, _ := uuid.Parse(req.PostID)
	
	// Kiểm tra bài đăng có tồn tại và đang mở không
	post, err := server.dbStore.GetExchangePost(c.Request.Context(), postID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("exchange post ID %s not found", req.PostID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	if post.Status != db.ExchangePostStatusOpen {
		err = fmt.Errorf("exchange post ID %s is not open", req.PostID)
		c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
		return
	}
	
	// Kiểm tra người dùng không tự đề xuất cho bài đăng của mình
	if post.UserID == offererID {
		err = fmt.Errorf("you cannot make an offer for your own post")
		c.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	// Kiểm tra số tiền bù và người bù tiền
	if req.PayerID != nil && req.CompensationAmount == nil {
		c.JSON(http.StatusUnprocessableEntity, errorResponse(errors.New("compensation amount is required when payer is specified")))
		return
	}
	
	if req.PayerID == nil && req.CompensationAmount != nil {
		c.JSON(http.StatusUnprocessableEntity, errorResponse(errors.New("payer is required when compensation amount is specified")))
		return
	}
	
	if req.CompensationAmount != nil && *req.CompensationAmount <= 0 {
		c.JSON(http.StatusForbidden, errorResponse(errors.New("compensation amount must be positive")))
		return
	}
	
	// Kiểm tra người bù tiền phải là người đề xuất hoặc người đăng bài
	if req.PayerID != nil && *req.PayerID != offererID && *req.PayerID != post.UserID {
		c.JSON(http.StatusForbidden, errorResponse(errors.New("payer must be either the poster or the offerer")))
		return
	}
	
	// Kiểm tra Gundam của người đề xuất có tồn tại không
	offererGundam, err := server.dbStore.GetGundamByID(c.Request.Context(), req.OffererGundamID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("offererGundam ID %d not found", req.OffererGundamID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	// Kiểm tra Gundam có thuộc về người đề xuất không
	if offererGundam.OwnerID != offererID {
		err = fmt.Errorf("offererGundam ID %d does not belong to user ID %s", req.OffererGundamID, offererID)
		c.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	if offererGundam.Status != db.GundamStatusInstore {
		err = fmt.Errorf("offererGundam ID %d is not available for exchange", req.OffererGundamID)
		c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
		return
	}
	
	// Kiểm tra Gundam của người đăng bài có tồn tại không
	posterGundam, err := server.dbStore.GetGundamByID(c.Request.Context(), req.PosterGundamID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("offererGundam ID %d not found", req.PosterGundamID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Kiểm tra Gundam có thuộc về người đăng bài không
	if posterGundam.OwnerID != post.UserID {
		err = fmt.Errorf("offererGundam ID %d does not belong to user ID %s", req.PosterGundamID, post.UserID)
		c.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	if posterGundam.Status != db.GundamStatusInstore {
		err = fmt.Errorf("offererGundam ID %d is not available for exchange", req.PosterGundamID)
		c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
		return
	}
	
}
