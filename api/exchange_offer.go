package api

import (
	"errors"
	"fmt"
	"net/http"
	
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/token"
	"github.com/katatrina/gundam-BE/internal/util"
	"github.com/katatrina/gundam-BE/internal/worker"
	"github.com/rs/zerolog/log"
)

type createExchangeOfferRequest struct {
	ExchangePostID     string  `json:"exchange_post_id" binding:"required,uuid"` // ID bài đăng trao đổi
	PosterGundamID     int64   `json:"poster_gundam_id" binding:"required"`      // ID Gundam của người đăng bài
	OffererGundamID    int64   `json:"offerer_gundam_id" binding:"required"`     // ID Gundam của người đề xuất
	PayerID            *string `json:"payer_id"`                                 // ID người bù tiền (poster_id hoặc offerer_id, hoặc null)
	CompensationAmount *int64  `json:"compensation_amount"`                      // Số tiền bù (null nếu không có bù tiền)
}

//	@Summary		Create an exchange offer
//	@Description	Create a 1-1 exchange offer with optional compensation.
//	@Tags			exchanges
//	@Accept			json
//	@Produce		json
//	@Security		accessToken
//	@Param			request	body		createExchangeOfferRequest		true	"Create exchange offer request"
//	@Success		201		{object}	db.CreateExchangeOfferTxResult	"Create exchange offer response"
//	@Router			/users/me/exchange-offers [post]
func (server *Server) createExchangeOffer(c *gin.Context) {
	// Lấy thông tin người dùng đã đăng nhập
	authPayload := c.MustGet(authorizationPayloadKey).(*token.Payload)
	offererID := authPayload.Subject
	
	var req createExchangeOfferRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	postID, err := uuid.Parse(req.ExchangePostID)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(fmt.Errorf("invalid post ID: %s", req.ExchangePostID)))
		return
	}
	
	// Kiểm tra bài đăng có tồn tại và đang mở không
	post, err := server.dbStore.GetExchangePost(c.Request.Context(), postID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("exchange post ID %s not found", req.ExchangePostID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	if post.Status != db.ExchangePostStatusOpen {
		err = fmt.Errorf("exchange post ID %s is not open for offers", req.ExchangePostID)
		c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
		return
	}
	
	// Kiểm tra người dùng không tự đề xuất cho bài đăng của mình
	if post.UserID == offererID {
		err = fmt.Errorf("you cannot make an offer to your own exchange post")
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
	
	// Chỉ kiểm tra số dư, không trừ tiền ngay. Tiền sẽ được trừ khi đề xuất được chấp nhận.
	if req.PayerID != nil && *req.PayerID == offererID && req.CompensationAmount != nil {
		wallet, err := server.dbStore.GetWalletByUserID(c.Request.Context(), offererID)
		if err != nil {
			if errors.Is(err, db.ErrRecordNotFound) {
				err = fmt.Errorf("wallet not found for user ID %s", offererID)
				c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
				return
			}
			
			c.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
		
		if wallet.Balance < *req.CompensationAmount {
			err = fmt.Errorf("insufficient balance for compensation: needed %d, available %d", *req.CompensationAmount, wallet.Balance)
			c.JSON(http.StatusForbidden, errorResponse(errors.New("insufficient balance for compensation")))
			return
		}
	}
	
	// Kiểm tra xem Gundam từ bài đăng có thuộc bài đăng này không
	_, err = server.dbStore.GetExchangePostItemByGundamID(c.Request.Context(), db.GetExchangePostItemByGundamIDParams{
		PostID:   postID,
		GundamID: req.PosterGundamID,
	})
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("gundam ID %d is not part of the exchange post %s", req.PosterGundamID, postID)
			c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
			return
		}
		
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Kiểm tra Gundam của người đề xuất có tồn tại không
	offererGundam, err := server.dbStore.GetGundamByID(c.Request.Context(), req.OffererGundamID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("gundam ID %d not found", req.OffererGundamID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Kiểm tra Gundam có thuộc về người đề xuất không
	if offererGundam.OwnerID != offererID {
		err = fmt.Errorf("user ID %s does not own gundam ID %d", offererID, req.OffererGundamID)
		c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
		return
	}
	
	// Kiểm tra Gundam có được phép trao đổi không
	if offererGundam.Status != db.GundamStatusInstore {
		err = fmt.Errorf("gundam ID %d is not available for exchange, current status: %s", req.OffererGundamID, offererGundam.Status)
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
	
	// TODO: Có thể kiểm tra Gundam đã tham gia các đề xuất khác chưa? Nếu có thì không cho phép tạo đề xuất mới.
	// Nhưng mà chỉ cần kiểm tra trạng thái của Gundam là được rồi.
	
	// Tạo đề xuất trao đổi
	result, err := server.dbStore.CreateExchangeOfferTx(c.Request.Context(), db.CreateExchangeOfferTxParams{
		PostID:             postID,
		OffererID:          offererID,
		PosterGundamID:     req.PosterGundamID,
		OffererGundamID:    req.OffererGundamID,
		CompensationAmount: req.CompensationAmount,
		PayerID:            req.PayerID,
	})
	if err != nil {
		if errors.Is(err, db.ErrExchangeOfferUnique) {
			err = fmt.Errorf("user ID %s already has an offer for exchange post ID %s", offererID, req.ExchangePostID)
			c.JSON(http.StatusForbidden, errorResponse(err))
			return
		}
		
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	opts := []asynq.Option{
		asynq.MaxRetry(3),
		asynq.Queue(worker.QueueCritical),
	}
	
	// Gửi thông báo cho người đăng bài về đề xuất trao đổi mới
	err = server.taskDistributor.DistributeTaskSendNotification(c.Request.Context(), &worker.PayloadSendNotification{
		RecipientID: post.UserID,
		Title:       "Đề xuất trao đổi mới",
		Message:     fmt.Sprintf("Có người muốn trao đổi Gundam lấy %s của bạn. Bạn có thể xem chi tiết đề xuất trong trang Trao đổi của tôi.", posterGundam.Name),
		Type:        "exchange",
		ReferenceID: result.Offer.ID.String(),
	}, opts...)
	if err != nil {
		log.Err(err).Msgf("failed to send notification to user ID %s", post.UserID)
	}
	
	c.JSON(http.StatusCreated, result)
}

// PostOfferURIParams định nghĩa tham số trên URI
type PostOfferURIParams struct {
	PostID  string `uri:"postID" binding:"required,uuid"`
	OfferID string `uri:"offerID" binding:"required,uuid"`
}

// requestNegotiationForOfferRequest là cấu trúc yêu cầu thương lượng
type requestNegotiationForOfferRequest struct {
	Note *string `json:"note"` // Ghi chú từ người yêu cầu thương lượng, không bắt buộc
}

//	@Summary		Request negotiation for an exchange offer
//	@Description	As a post owner, request negotiation with an offerer.
//	@Tags			exchanges
//	@Accept			json
//	@Produce		json
//	@Security		accessToken
//	@Param			postID	path		string									true	"Exchange Post ID"
//	@Param			offerID	path		string									true	"Exchange Offer ID"
//	@Param			request	body		requestNegotiationForOfferRequest		false	"Negotiation request"
//	@Success		200		{object}	db.RequestNegotiationForOfferTxResult	"Negotiation request response"
//	@Router			/users/me/exchange-posts/{postID}/offers/{offerID}/negotiate [patch]
func (server *Server) requestNegotiationForOffer(c *gin.Context) {
	// Lấy thông tin người dùng đã đăng nhập
	authPayload := c.MustGet(authorizationPayloadKey).(*token.Payload)
	userID := authPayload.Subject
	
	// Bind các tham số từ URI
	var uriParams PostOfferURIParams
	if err := c.ShouldBindUri(&uriParams); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	// Parse UUID từ string
	postID, err := uuid.Parse(uriParams.PostID)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(fmt.Errorf("invalid post ID: %s", uriParams.PostID)))
		return
	}
	
	offerID, err := uuid.Parse(uriParams.OfferID)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(fmt.Errorf("invalid offer ID: %s", uriParams.OfferID)))
		return
	}
	
	// Đọc request body
	var req requestNegotiationForOfferRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	// -------------------
	// PHẦN 1: Kiểm tra business rules
	// -------------------
	
	// 1. Kiểm tra bài đăng tồn tại và người dùng là chủ sở hữu
	post, err := server.dbStore.GetExchangePost(c, postID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("exchange post ID %s not found", uriParams.PostID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	if post.UserID != userID {
		err = fmt.Errorf("user ID %s is not the owner of exchange post ID %s", userID, uriParams.PostID)
		c.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	if post.Status != db.ExchangePostStatusOpen {
		err = fmt.Errorf("exchange post ID %s is not open for negotiation", uriParams.PostID)
		c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
		return
	}
	
	// 2. Kiểm tra đề xuất tồn tại và thuộc về bài đăng này
	offer, err := server.dbStore.GetExchangeOffer(c, offerID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("exchange offer ID %s not found", uriParams.OfferID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	if offer.PostID != postID {
		err = fmt.Errorf("exchange offer ID %s does not belong to exchange post ID %s", uriParams.OfferID, uriParams.PostID)
		c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
		return
	}
	
	// 3. Kiểm tra số lần thương lượng đã sử dụng
	if offer.NegotiationsCount >= offer.MaxNegotiations {
		err = fmt.Errorf("maximum number of negotiations reached for exchange offer ID %s, current count: %d, max: %d", uriParams.OfferID, offer.NegotiationsCount, offer.MaxNegotiations)
		c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
		return
	}
	
	// 4. Kiểm tra xem hiện tại có đang yêu cầu thương lượng không
	if offer.NegotiationRequested {
		err = fmt.Errorf("negotiation already requested for exchange offer ID %s", uriParams.OfferID)
		c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
		return
	}
	
	// -------------------
	// PHẦN 2: Xử lý transaction để cập nhật dữ liệu
	// -------------------
	
	// Thực hiện transaction
	result, err := server.dbStore.RequestNegotiationForOfferTx(c, db.RequestNegotiationForOfferTxParams{
		OfferID: offerID,
		UserID:  userID,
		Note:    req.Note,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Gửi thông báo cho người đề xuất về yêu cầu thương lượng
	opts := []asynq.Option{
		asynq.MaxRetry(3),
		asynq.Queue(worker.QueueCritical),
	}
	
	// Tạo thông báo ngắn gọn nhưng đủ thông tin
	notificationMessage := fmt.Sprintf(
		"Chủ bài đăng '%s' đã yêu cầu thương lượng cho đề xuất trao đổi Gundam của bạn.",
		util.TruncateContent(post.Content, 20), // Hàm rút gọn tiêu đề nếu quá dài
	)
	
	err = server.taskDistributor.DistributeTaskSendNotification(c.Request.Context(), &worker.PayloadSendNotification{
		RecipientID: offer.OffererID,
		Title:       "Yêu cầu thương lượng Gundam",
		Message:     notificationMessage,
		Type:        "exchange",
		ReferenceID: result.Offer.ID.String(),
	}, opts...)
	if err != nil {
		log.Err(err).
			Str("offerID", result.Offer.ID.String()).
			Str("postID", post.ID.String()).
			Msgf("failed to send notification to user ID %s", offer.OffererID)
	}
	
	// Trả về kết quả
	c.JSON(http.StatusOK, result)
}
