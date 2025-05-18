package api

import (
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/token"
	"github.com/katatrina/gundam-BE/internal/util"
	"github.com/katatrina/gundam-BE/internal/worker"
	"github.com/rs/zerolog/log"
)

type createExchangePostRequest struct {
	Content     string                  `form:"content" binding:"required"`                 // Nội dung bài post
	PostImages  []*multipart.FileHeader `form:"post_images" binding:"required,max=5,min=1"` // Ảnh bài post
	PostItemIDs []int64                 `form:"post_item_id" binding:"required,min=1"`      // ID của các Gundam mà chủ bài post cho phép trao đổi
}

//	@Summary		Create a new exchange post
//	@Description	Create a new exchange post.
//	@Tags			exchanges
//	@Accept			multipart/form-data
//	@Produce		json
//	@Security		accessToken
//	@Param			request	body		createExchangePostRequest		true	"Create exchange post request"
//	@Success		201		{object}	db.CreateExchangePostTxResult	"Create exchange post response"
//	@Router			/users/me/exchange-posts [post]
func (server *Server) createExchangePost(c *gin.Context) {
	authPayload := c.MustGet(authorizationPayloadKey).(*token.Payload)
	userID := authPayload.Subject
	
	_, err := server.dbStore.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("user ID %s not found", userID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	var req createExchangePostRequest
	if err := c.ShouldBindWith(&req, binding.FormMultipart); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	for _, itemID := range req.PostItemIDs {
		gundam, err := server.dbStore.GetGundamByID(c.Request.Context(), itemID)
		if err != nil {
			if errors.Is(err, db.ErrRecordNotFound) {
				err = fmt.Errorf("gundam ID %d not found", itemID)
				c.JSON(http.StatusNotFound, errorResponse(err))
				return
			}
			
			c.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
		
		// Kiểm tra người dùng có phải là chủ sở hữu của Gundam không
		if gundam.OwnerID != userID {
			err = fmt.Errorf("user ID %s does not own gundam ID %d", userID, itemID)
			c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
			return
		}
		
		// Kiểm tra trạng thái hiện tại của Gundam có được phép trao đổi không
		if gundam.Status != db.GundamStatusInstore {
			err = fmt.Errorf("gundam ID %d is not available for exchange", itemID)
			c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
			return
		}
	}
	
	result, err := server.dbStore.CreateExchangePostTx(c.Request.Context(), db.CreateExchangePostTxParams{
		UserID:           userID,
		Content:          req.Content,
		PostImages:       req.PostImages,
		PostItemIDs:      req.PostItemIDs,
		UploadImagesFunc: server.uploadFileToCloudinary,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	c.JSON(http.StatusCreated, result)
}

//	@Summary		List user's exchange posts
//	@Description	List all exchange posts created by the current authenticated user.
//	@Tags			exchanges
//	@Produce		json
//	@Security		accessToken
//	@Param			status	query	string						false	"Filter by status (open, closed)"
//	@Success		200		{array}	db.UserExchangePostDetails	"List of user's exchange posts"
//	@Router			/users/me/exchange-posts [get]
func (server *Server) listUserExchangePosts(c *gin.Context) {
	// Lấy thông tin người dùng đã đăng nhập
	authPayload := c.MustGet(authorizationPayloadKey).(*token.Payload)
	userID := authPayload.Subject
	
	status := c.Query("status")
	
	// Lấy danh sách bài đăng trao đổi của người dùng
	arg := db.ListUserExchangePostsParams{
		UserID: userID,
	}
	
	if status != "" {
		if err := db.IsValidExchangePostStatus(status); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse(err))
			return
		}
		
		arg.Status = db.NullExchangePostStatus{
			ExchangePostStatus: db.ExchangePostStatus(status),
			Valid:              true,
		}
	}
	
	posts, err := server.dbStore.ListUserExchangePosts(c.Request.Context(), arg)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Xây dựng kết quả chi tiết cho từng bài đăng
	result := make([]db.UserExchangePostDetails, 0, len(posts))
	for _, post := range posts {
		postDetails := db.UserExchangePostDetails{
			ExchangePost: post,
		}
		
		// Lấy danh sách các item trong bài đăng
		items, err := server.dbStore.ListExchangePostItems(c.Request.Context(), post.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
		
		postItemDetails := make([]db.GundamDetails, 0, len(items))
		
		// Lặp qua từng item trong bài đăng để lấy thông tin chi tiết của Gundam
		for _, item := range items {
			// Lấy toàn bộ thông tin của một con Gundam
			gundamDetails, err := server.dbStore.GetGundamDetailsByID(c.Request.Context(), nil, item.GundamID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, errorResponse(err))
				return
			}
			
			postItemDetails = append(postItemDetails, gundamDetails)
		}
		postDetails.ExchangePostItems = postItemDetails
		
		// Lấy danh sách đề xuất của bài đăng
		offers, err := server.dbStore.ListExchangeOffers(c.Request.Context(), post.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
		
		// Lấy thông tin chi tiết của mỗi đề xuất
		offerInfos := make([]db.ExchangeOfferInfo, 0, len(offers))
		for _, offer := range offers {
			offerInfo := db.ExchangeOfferInfo{
				ID:                   offer.ID,
				PostID:               post.ID,
				PayerID:              offer.PayerID,
				CompensationAmount:   offer.CompensationAmount,
				Note:                 offer.Note,
				NegotiationsCount:    offer.NegotiationsCount,
				MaxNegotiations:      offer.MaxNegotiations,
				NegotiationRequested: offer.NegotiationRequested,
				LastNegotiationAt:    offer.LastNegotiationAt,
				CreatedAt:            offer.CreatedAt,
				UpdatedAt:            offer.UpdatedAt,
			}
			
			// Lấy thông tin người đề xuất
			offerer, err := server.dbStore.GetUserByID(c.Request.Context(), offer.OffererID)
			if err != nil {
				if errors.Is(err, db.ErrRecordNotFound) {
					err = fmt.Errorf("offerer user ID %s not found", offer.OffererID)
					c.JSON(http.StatusNotFound, errorResponse(err))
					return
				}
				
				c.JSON(http.StatusInternalServerError, errorResponse(err))
				return
			}
			offerInfo.Offerer = offerer
			
			// Lấy danh sách gundam của người đề xuất
			offererItems, err := server.dbStore.ListExchangeOfferItems(c.Request.Context(), db.ListExchangeOfferItemsParams{
				OfferID:      offer.ID,
				IsFromPoster: util.BoolPointer(false),
			})
			if err != nil {
				c.JSON(http.StatusInternalServerError, errorResponse(err))
				return
			}
			
			// Lấy thông tin chi tiết gundam
			offererGundams := make([]db.GundamDetails, 0, len(offererItems))
			for _, item := range offererItems {
				// Lấy thông tin chi tiết Gundam của người đề xuất
				gundamDetails, err := server.dbStore.GetGundamDetailsByID(c.Request.Context(), nil, item.GundamID)
				if err != nil {
					c.JSON(http.StatusInternalServerError, errorResponse(err))
					return
				}
				offererGundams = append(offererGundams, gundamDetails)
			}
			offerInfo.OffererExchangeItems = offererGundams
			
			// Lấy Gundam của người đăng bài mà người đề xuất muốn
			posterItems, err := server.dbStore.ListExchangeOfferItems(c.Request.Context(), db.ListExchangeOfferItemsParams{
				OfferID:      offer.ID,
				IsFromPoster: util.BoolPointer(true),
			})
			if err != nil {
				c.JSON(http.StatusInternalServerError, errorResponse(err))
				return
			}
			
			posterGundams := make([]db.GundamDetails, 0, len(posterItems))
			for _, item := range posterItems {
				// Lấy thông tin chi tiết Gundam của người đăng bài
				gundamDetails, err := server.dbStore.GetGundamDetailsByID(c.Request.Context(), nil, item.GundamID)
				if err != nil {
					c.JSON(http.StatusInternalServerError, errorResponse(err))
					return
				}
				posterGundams = append(posterGundams, gundamDetails)
			}
			offerInfo.PosterExchangeItems = posterGundams
			
			// Lấy thông tin các ghi chú thương lượng (nếu có)
			notes, err := server.dbStore.ListExchangeOfferNotes(c.Request.Context(), offer.ID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, errorResponse(err))
				return
			}
			offerInfo.NegotiationNotes = notes
			
			offerInfos = append(offerInfos, offerInfo)
		}
		
		postDetails.Offers = offerInfos
		result = append(result, postDetails)
	}
	
	c.JSON(http.StatusOK, result)
}

//	@Summary		List all open exchange posts
//	@Description	List all open exchange posts.
//	@Tags			exchanges
//	@Produce		json
//	@Success		200	{array}	db.OpenExchangePostInfo	"List of open exchange posts"
//	@Router			/exchange-posts [get]
func (server *Server) listOpenExchangePosts(c *gin.Context) {
	// var userID string
	//
	// // Kiểm tra có người dùng đăng nhập hay không
	// payload, exists := c.Get(authorizationPayloadKey)
	// if exists && payload != nil {
	// 	authPayload, ok := payload.(*token.Payload)
	// 	if ok && authPayload != nil {
	// 		userID = authPayload.Subject
	// 	}
	// }
	
	// Lấy danh sách các bài đăng trao đổi đang mở
	posts, err := server.dbStore.ListExchangePosts(c.Request.Context(), db.NullExchangePostStatus{
		ExchangePostStatus: db.ExchangePostStatusOpen,
		Valid:              true,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	result := make([]db.OpenExchangePostInfo, 0, len(posts))
	
	for _, post := range posts {
		postInfo := db.OpenExchangePostInfo{
			ExchangePost: post,
		}
		
		// Lấy thông tin người đăng
		poster, err := server.dbStore.GetUserByID(c.Request.Context(), post.UserID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
		postInfo.Poster = poster
		
		// Lấy danh sách các item trong bài đăng
		items, err := server.dbStore.ListExchangePostItems(c.Request.Context(), post.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
		
		postItemDetails := make([]db.GundamDetails, 0, len(items))
		
		// Lặp qua từng item trong bài đăng để lấy thông tin chi tiết của Gundam
		for _, item := range items {
			// Lấy toàn bộ thông tin chi tiết của Gundam
			gundamDetails, err := server.dbStore.GetGundamDetailsByID(c.Request.Context(), nil, item.GundamID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, errorResponse(err))
				return
			}
			
			postItemDetails = append(postItemDetails, gundamDetails)
		}
		postInfo.ExchangePostItems = postItemDetails
		
		// Đếm số lượng offer của bài đăng
		offerCount, err := server.dbStore.CountExchangeOffers(c.Request.Context(), post.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
		postInfo.OfferCount = offerCount
		
		// Lấy offer của người dùng đã đăng nhập (nếu có)
		// if userID != "" && userID != post.User { // Kiểm tra người dùng không phải là người đăng bài
		// 	offer, err := server.dbStore.GetUserExchangeOfferForPost(c.Request.Context(), db.GetUserExchangeOfferForPostParams{
		// 		PostID:    post.OfferID,
		// 		OffererID: userID,
		// 	})
		// 	if err != nil {
		// 		if !errors.Is(err, db.ErrRecordNotFound) {
		// 			c.JSON(http.StatusInternalServerError, errorResponse(err))
		// 			return
		// 		}
		// 		// Nếu không tìm thấy offer, thì authenticatedUserOffer sẽ là nil
		// 	} else {
		// 		postInfo.AuthenticatedUserOffer = &offer
		//
		// 		// Lấy thông tin các Gundam trong offer (chỉ từ người đề xuất)
		// 		offerItemsFromOfferer, err := server.dbStore.ListExchangeOfferItems(c.Request.Context(), db.ListExchangeOfferItemsParams{
		// 			OfferID:      offer.OfferID,
		// 			IsFromPoster: util.BoolPointer(false),
		// 		})
		// 		if err != nil {
		// 			c.JSON(http.StatusInternalServerError, errorResponse(err))
		// 			return
		// 		}
		// 		offerGundamDetails := make([]db.GundamDetails, 0, len(offerItemsFromOfferer))
		// 		// Lặp qua từng item trong offer để lấy thông tin chi tiết của Gundam
		// 		for _, item := range offerItemsFromOfferer {
		// 			// Lấy thông tin Gundam của mỗi item
		// 			gundam, err := server.dbStore.GetGundamByID(c.Request.Context(), item.GundamID)
		// 			if err != nil {
		// 				if errors.Is(err, db.ErrRecordNotFound) {
		// 					err = fmt.Errorf("gundam ID %d not found", item.GundamID)
		// 					c.JSON(http.StatusNotFound, errorResponse(err))
		// 					return
		// 				}
		//
		// 				c.JSON(http.StatusInternalServerError, errorResponse(err))
		// 				return
		// 			}
		//
		// 			// Lấy thông tin grade của Gundam
		// 			grade, err := server.dbStore.GetGradeByID(c.Request.Context(), gundam.GradeID)
		// 			if err != nil {
		// 				if errors.Is(err, db.ErrRecordNotFound) {
		// 					err = fmt.Errorf("grade IDf %d not found", gundam.GradeID)
		// 					c.JSON(http.StatusNotFound, errorResponse(err))
		// 					return
		// 				}
		//
		// 				c.JSON(http.StatusInternalServerError, errorResponse(err))
		// 				return
		// 			}
		//
		// 			// Lấy ảnh chính của Gundam
		// 			primaryImageURL, err := server.dbStore.GetGundamPrimaryImageURL(c.Request.Context(), gundam.OfferID)
		// 			if err != nil {
		// 				if errors.Is(err, db.ErrRecordNotFound) {
		// 					err = fmt.Errorf("primary image of gundam ID %d not found", gundam.OfferID)
		// 					c.JSON(http.StatusNotFound, errorResponse(err))
		// 					return
		// 				}
		//
		// 				c.JSON(http.StatusInternalServerError, errorResponse(err))
		// 				return
		// 			}
		//
		// 			// Lấy ảnh phụ của Gundam
		// 			secondaryImageURLs, err := server.dbStore.GetGundamSecondaryImageURLs(c.Request.Context(), gundam.OfferID)
		// 			if err != nil {
		// 				if errors.Is(err, db.ErrRecordNotFound) {
		// 					err = fmt.Errorf("secondary images of gundam ID %d not found", gundam.OfferID)
		// 					c.JSON(http.StatusNotFound, errorResponse(err))
		// 					return
		// 				}
		//
		// 				c.JSON(http.StatusInternalServerError, errorResponse(err))
		// 				return
		// 			}
		//
		// 			// Lấy thông tin phụ kiện của Gundam
		// 			accessories, err := server.dbStore.GetGundamAccessories(c.Request.Context(), gundam.OfferID)
		// 			if err != nil {
		// 				if errors.Is(err, db.ErrRecordNotFound) {
		// 					err = fmt.Errorf("accessories of gundam ID %d not found", gundam.OfferID)
		// 					c.JSON(http.StatusNotFound, errorResponse(err))
		// 					return
		// 				}
		//
		// 				c.JSON(http.StatusInternalServerError, errorResponse(err))
		// 				return
		// 			}
		//
		// 			// Chuyển đổi phụ kiện sang định dạng DTO
		// 			accessoryDTOs := make([]db.GundamAccessoryDTO, len(accessories))
		// 			for i, accessory := range accessories {
		// 				accessoryDTOs[i] = db.ConvertGundamAccessoryToDTO(accessory)
		// 			}
		//
		// 			// Tạo đối tượng GundamDetails
		// 			detail := db.GundamDetails{
		// 				OfferID:                   gundam.OfferID,
		// 				OwnerID:              gundam.OwnerID,
		// 				Name:                 gundam.Name,
		// 				Slug:                 gundam.Slug,
		// 				Grade:                grade.DisplayName,
		// 				Series:               gundam.Series,
		// 				PartsTotal:           gundam.PartsTotal,
		// 				Material:             gundam.Material,
		// 				Version:              gundam.Version,
		// 				Quantity:             gundam.Quantity,
		// 				Condition:            string(gundam.Condition),
		// 				ConditionDescription: gundam.ConditionDescription,
		// 				Manufacturer:         gundam.Manufacturer,
		// 				Weight:               gundam.Weight,
		// 				Scale:                string(gundam.Scale),
		// 				Description:          gundam.Description,
		// 				Price:                gundam.Price,
		// 				ReleaseYear:          gundam.ReleaseYear,
		// 				Status:               string(gundam.Status),
		// 				Accessories:          accessoryDTOs,
		// 				PrimaryImageURL:      primaryImageURL,
		// 				SecondaryImageURLs:   secondaryImageURLs,
		// 				CreatedAt:            gundam.CreatedAt,
		// 				UpdatedAt:            gundam.UpdatedAt,
		// 			}
		//
		// 			offerGundamDetails = append(offerGundamDetails, detail)
		// 		}
		// 		postInfo.AuthenticatedUserOfferItems = offerGundamDetails
		//
		// 		// Lấy thông tin các Gundam từ người đăng mà người đề xuất muốn nhận
		// 		offerItemsFromPoster, err := server.dbStore.ListExchangeOfferItems(c.Request.Context(), db.ListExchangeOfferItemsParams{
		// 			OfferID:      offer.OfferID,
		// 			IsFromPoster: util.BoolPointer(true),
		// 		})
		// 		if err != nil {
		// 			c.JSON(http.StatusInternalServerError, errorResponse(err))
		// 			return
		// 		}
		// 		wantedGundamDetails := make([]db.GundamDetails, 0, len(offerItemsFromOfferer))
		// 		// Lặp qua từng item trong offer để lấy thông tin chi tiết của Gundam
		// 		for _, item := range offerItemsFromPoster {
		// 			// Lấy thông tin Gundam của mỗi item
		// 			gundam, err := server.dbStore.GetGundamByID(c.Request.Context(), item.GundamID)
		// 			if err != nil {
		// 				if errors.Is(err, db.ErrRecordNotFound) {
		// 					err = fmt.Errorf("gundam ID %d not found", item.GundamID)
		// 					c.JSON(http.StatusNotFound, errorResponse(err))
		// 					return
		// 				}
		//
		// 				c.JSON(http.StatusInternalServerError, errorResponse(err))
		// 				return
		// 			}
		//
		// 			// Lấy thông tin grade của Gundam
		// 			grade, err := server.dbStore.GetGradeByID(c.Request.Context(), gundam.GradeID)
		// 			if err != nil {
		// 				if errors.Is(err, db.ErrRecordNotFound) {
		// 					err = fmt.Errorf("grade ID %d not found", gundam.GradeID)
		// 					c.JSON(http.StatusNotFound, errorResponse(err))
		// 					return
		// 				}
		//
		// 				c.JSON(http.StatusInternalServerError, errorResponse(err))
		// 				return
		// 			}
		//
		// 			// Lấy ảnh chính của Gundam
		// 			primaryImageURL, err := server.dbStore.GetGundamPrimaryImageURL(c.Request.Context(), gundam.OfferID)
		// 			if err != nil {
		// 				if errors.Is(err, db.ErrRecordNotFound) {
		// 					err = fmt.Errorf("primary image of gundam ID %d not found", gundam.OfferID)
		// 					c.JSON(http.StatusNotFound, errorResponse(err))
		// 					return
		// 				}
		//
		// 				c.JSON(http.StatusInternalServerError, errorResponse(err))
		// 				return
		// 			}
		//
		// 			// Lấy ảnh phụ của Gundam
		// 			secondaryImageURLs, err := server.dbStore.GetGundamSecondaryImageURLs(c.Request.Context(), gundam.OfferID)
		// 			if err != nil {
		// 				c.JSON(http.StatusInternalServerError, errorResponse(err))
		// 				return
		// 			}
		//
		// 			// Lấy thông tin phụ kiện của Gundam
		// 			accessories, err := server.dbStore.GetGundamAccessories(c.Request.Context(), gundam.OfferID)
		// 			if err != nil {
		// 				c.JSON(http.StatusInternalServerError, errorResponse(err))
		// 				return
		// 			}
		//
		// 			// Chuyển đổi phụ kiện sang định dạng DTO
		// 			accessoryDTOs := make([]db.GundamAccessoryDTO, len(accessories))
		// 			for i, accessory := range accessories {
		// 				accessoryDTOs[i] = db.ConvertGundamAccessoryToDTO(accessory)
		// 			}
		//
		// 			// Tạo đối tượng GundamDetails
		// 			detail := db.GundamDetails{
		// 				OfferID:                   gundam.OfferID,
		// 				OwnerID:              gundam.OwnerID,
		// 				Name:                 gundam.Name,
		// 				Slug:                 gundam.Slug,
		// 				Grade:                grade.DisplayName,
		// 				Series:               gundam.Series,
		// 				PartsTotal:           gundam.PartsTotal,
		// 				Material:             gundam.Material,
		// 				Version:              gundam.Version,
		// 				Quantity:             gundam.Quantity,
		// 				Condition:            string(gundam.Condition),
		// 				ConditionDescription: gundam.ConditionDescription,
		// 				Manufacturer:         gundam.Manufacturer,
		// 				Weight:               gundam.Weight,
		// 				Scale:                string(gundam.Scale),
		// 				Description:          gundam.Description,
		// 				Price:                gundam.Price,
		// 				ReleaseYear:          gundam.ReleaseYear,
		// 				Status:               string(gundam.Status),
		// 				Accessories:          accessoryDTOs,
		// 				PrimaryImageURL:      primaryImageURL,
		// 				SecondaryImageURLs:   secondaryImageURLs,
		// 				CreatedAt:            gundam.CreatedAt,
		// 				UpdatedAt:            gundam.UpdatedAt,
		// 			}
		//
		// 			wantedGundamDetails = append(wantedGundamDetails, detail)
		// 		}
		// 		postInfo.AuthenticatedUserWantedItems = wantedGundamDetails
		// 	}
		// }
		
		result = append(result, postInfo)
	}
	
	c.JSON(http.StatusOK, result)
}

//	@Summary		Delete an exchange post
//	@Description	Deletes an exchange post and resets the status of associated gundams. Only the post owner can delete it.
//	@Tags			exchanges
//	@Produce		json
//	@Security		accessToken
//	@Param			id	path		string							true	"Exchange Post ID"
//	@Success		200	{object}	db.DeleteExchangePostTxResult	"Delete exchange post response"
//	@Router			/users/me/exchange-posts/{id} [delete]
func (server *Server) deleteExchangePost(c *gin.Context) {
	// Lấy thông tin người dùng đã đăng nhập
	authPayload := c.MustGet(authorizationPayloadKey).(*token.Payload)
	userID := authPayload.Subject
	
	// Lấy ID bài đăng từ URL
	postIDStr := c.Param("postID")
	postID, err := uuid.Parse(postIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(fmt.Errorf("invalid post ID format: %s", postIDStr)))
		return
	}
	
	// Kiểm tra bài đăng có tồn tại không
	post, err := server.dbStore.GetExchangePost(c.Request.Context(), postID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, errorResponse(fmt.Errorf("exchange post ID %s not found", postIDStr)))
			return
		}
		
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Kiểm tra người dùng có quyền xóa bài đăng không
	if post.UserID != userID {
		err = fmt.Errorf("user ID %s is not the owner of exchange post ID %s", userID, post.ID)
		c.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	// TODO: Cân nhắc không cho phép xóa bài đăng nếu có đề xuất đang được thương lượng
	
	// Xóa bài đăng và các thông tin liên quan
	result, err := server.dbStore.DeleteExchangePostTx(c.Request.Context(), db.DeleteExchangePostTxParams{
		PostID: postID,
		UserID: userID,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Thông báo cho những người đã gửi offer về việc bài đăng đã bị xóa
	for _, offer := range result.DeletedExchangePostOffers {
		err = server.taskDistributor.DistributeTaskSendNotification(c.Request.Context(), &worker.PayloadSendNotification{
			RecipientID: offer.OffererID,
			Title:       "Bài đăng trao đổi đã bị xóa",
			Message:     "Bài đăng trao đổi mà bạn đề xuất đã bị xóa. Gundam của bạn đã được đưa trở lại trạng thái sẵn có.",
			Type:        "exchange",
			ReferenceID: offer.ID.String(),
		}, []asynq.Option{
			asynq.MaxRetry(3),
			asynq.Queue(worker.QueueCritical)}...)
		if err != nil {
			log.Info().Msgf("failed to send notification to user %s: %v", offer.OffererID, err)
		}
	}
	
	c.JSON(http.StatusOK, result)
}

// @Summary		Get user's exchange post details
// @Description	Get detailed information about a specific exchange post owned by the authenticated user, including items and offers.
// @Tags			exchanges
// @Produce		json
// @Security		accessToken
// @Param			postID	path		string						true	"Exchange Post ID"
// @Param			status	query		string						false	"Filter by status (open, closed)"
// @Success		200		{object}	db.UserExchangePostDetails	"User exchange post details"
// @Failure		400		{object}	error						"Invalid post ID or status"
// @Failure		404		{object}	error						"Post not found"
// @Router			/users/me/exchange-posts/{postID} [get]
func (server *Server) getUserExchangePost(c *gin.Context) {
	// Lấy thông tin người dùng đã đăng nhập
	authPayload := c.MustGet(authorizationPayloadKey).(*token.Payload)
	userID := authPayload.Subject
	
	// Lấy ID bài đăng từ URL
	postIDStr := c.Param("postID")
	postID, err := uuid.Parse(postIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(fmt.Errorf("invalid post ID: %s", postIDStr)))
		return
	}
	
	status := c.Query("status")
	
	arg := db.GetUserExchangePostParams{
		PostID: postID,
		UserID: userID,
	}
	
	if status != "" {
		if err := db.IsValidExchangePostStatus(status); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse(err))
			return
		}
		
		arg.Status = db.NullExchangePostStatus{
			ExchangePostStatus: db.ExchangePostStatus(status),
			Valid:              true,
		}
	}
	
	if status != "" {
		if err := db.IsValidExchangePostStatus(status); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse(err))
			return
		}
		
		arg.Status = db.NullExchangePostStatus{
			ExchangePostStatus: db.ExchangePostStatus(status),
			Valid:              true,
		}
	}
	
	post, err := server.dbStore.GetUserExchangePost(c.Request.Context(), arg)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("exchange post ID %s not found", postIDStr)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	var postDetails db.UserExchangePostDetails
	postDetails.ExchangePost = post
	
	// Lấy danh sách các item trong bài đăng
	items, err := server.dbStore.ListExchangePostItems(c.Request.Context(), post.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	postItemDetails := make([]db.GundamDetails, 0, len(items))
	
	// Lặp qua từng item trong bài đăng để lấy thông tin chi tiết của Gundam
	for _, item := range items {
		// Lấy toàn bộ thông tin của một con Gundam
		gundamDetails, err := server.dbStore.GetGundamDetailsByID(c.Request.Context(), nil, item.GundamID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
		
		postItemDetails = append(postItemDetails, gundamDetails)
	}
	postDetails.ExchangePostItems = postItemDetails
	
	// Lấy số lượng đề xuất của bài đăng
	offerCount, err := server.dbStore.CountExchangeOffers(c.Request.Context(), post.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	postDetails.OfferCount = offerCount
	
	// Lấy danh sách đề xuất của bài đăng
	offers, err := server.dbStore.ListExchangeOffers(c.Request.Context(), post.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Lấy thông tin chi tiết của mỗi đề xuất
	offerInfos := make([]db.ExchangeOfferInfo, 0, len(offers))
	for _, offer := range offers {
		offerInfo := db.ExchangeOfferInfo{
			ID:                   offer.ID,
			PostID:               post.ID,
			PayerID:              offer.PayerID,
			CompensationAmount:   offer.CompensationAmount,
			Note:                 offer.Note,
			NegotiationsCount:    offer.NegotiationsCount,
			MaxNegotiations:      offer.MaxNegotiations,
			NegotiationRequested: offer.NegotiationRequested,
			LastNegotiationAt:    offer.LastNegotiationAt,
			CreatedAt:            offer.CreatedAt,
			UpdatedAt:            offer.UpdatedAt,
		}
		// Lấy thông tin người đề xuất
		offerer, err := server.dbStore.GetUserByID(c.Request.Context(), offer.OffererID)
		if err != nil {
			if errors.Is(err, db.ErrRecordNotFound) {
				err = fmt.Errorf("offerer user ID %s not found", offer.OffererID)
				c.JSON(http.StatusNotFound, errorResponse(err))
				return
			}
			
			c.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
		offerInfo.Offerer = offerer
		
		// Lấy danh sách gundam của người đề xuất
		offererItems, err := server.dbStore.ListExchangeOfferItems(c.Request.Context(), db.ListExchangeOfferItemsParams{
			OfferID:      offer.ID,
			IsFromPoster: util.BoolPointer(false),
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
		
		// Lấy thông tin chi tiết gundam
		offererGundams := make([]db.GundamDetails, 0, len(offererItems))
		for _, item := range offererItems {
			// Lấy thông tin chi tiết Gundam của người đề xuất
			gundamDetails, err := server.dbStore.GetGundamDetailsByID(c.Request.Context(), nil, item.GundamID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, errorResponse(err))
				return
			}
			offererGundams = append(offererGundams, gundamDetails)
		}
		offerInfo.OffererExchangeItems = offererGundams
		
		// Lấy Gundam của người đăng bài mà người đề xuất muốn
		posterItems, err := server.dbStore.ListExchangeOfferItems(c.Request.Context(), db.ListExchangeOfferItemsParams{
			OfferID:      offer.ID,
			IsFromPoster: util.BoolPointer(true),
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
		posterGundams := make([]db.GundamDetails, 0, len(posterItems))
		for _, item := range posterItems {
			// Lấy thông tin chi tiết Gundam của người đăng bài
			gundamDetails, err := server.dbStore.GetGundamDetailsByID(c.Request.Context(), nil, item.GundamID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, errorResponse(err))
				return
			}
			posterGundams = append(posterGundams, gundamDetails)
		}
		offerInfo.PosterExchangeItems = posterGundams
		
		// Lấy thông tin các ghi chú thương lượng (nếu có)
		notes, err := server.dbStore.ListExchangeOfferNotes(c.Request.Context(), offer.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
		offerInfo.NegotiationNotes = notes
		
		offerInfos = append(offerInfos, offerInfo)
	}
	
	postDetails.Offers = offerInfos
	
	c.JSON(http.StatusOK, postDetails)
}
