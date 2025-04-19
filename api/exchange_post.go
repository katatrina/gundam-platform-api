package api

import (
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/token"
)

type createExchangePostRequest struct {
	Content     string                  `form:"content" binding:"required"`
	PostImages  []*multipart.FileHeader `form:"post_images" binding:"required"`
	PostItemIDs []int64                 `form:"post_item_id" binding:"required"`
}

//	@Summary		Create a new exchange post
//	@Description	Create a new exchange post.
//	@Tags			exchanges
//	@Accept			json
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
	
	// Kiểm tra xem người dùng có đang sở hữu các Gundam trong post_item_ids không
	// và có thể thêm các Gundam này vào bài post không.
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
		
		if gundam.OwnerID != userID {
			err = fmt.Errorf("user ID %s does not own gundam ID %d", userID, itemID)
			c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
			return
		}
		
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

func (server *Server) listUserExchangePosts(c *gin.Context) {

}

func (server *Server) listOpenExchangePosts(c *gin.Context) {
	var userID string
	
	// Kiểm tra xem có payload xác thực không
	payload, exists := c.Get(authorizationPayloadKey)
	if exists && payload != nil {
		authPayload, ok := payload.(*token.Payload)
		if ok && authPayload != nil {
			userID = authPayload.Subject
		}
	}
	
	posts, err := server.dbStore.ListExchangePosts(c.Request.Context(), db.NullExchangePostStatus{
		ExchangePostStatus: db.ExchangePostStatusOpen,
		Valid:              true,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	result := make([]db.ExchangePostInfo, 0, len(posts))
	
	for _, post := range posts {
		postInfo := db.ExchangePostInfo{
			ExchangePost: post,
		}
		
		// Lấy thông tin người đăng
		poster, err := server.dbStore.GetUserByID(c.Request.Context(), post.UserID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
		postInfo.Poster = poster
		
		items, err := server.dbStore.ListExchangePostItems(c.Request.Context(), post.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
		
		gundamDetails := make([]db.GundamDetails, 0, len(items))
		
		for _, item := range items {
			gundam, err := server.dbStore.GetGundamByID(c.Request.Context(), item.GundamID)
			if err != nil {
				if errors.Is(err, db.ErrRecordNotFound) {
					err = fmt.Errorf("gundam ID %d not found", item.GundamID)
					c.JSON(http.StatusNotFound, errorResponse(err))
					return
				}
				
				c.JSON(http.StatusInternalServerError, errorResponse(err))
				return
			}
			
			// Lấy thông tin grade
			grade, err := server.dbStore.GetGradeByID(c.Request.Context(), gundam.GradeID)
			if err != nil {
				if errors.Is(err, db.ErrRecordNotFound) {
					err = fmt.Errorf("grade ID %d not found", gundam.GradeID)
					c.JSON(http.StatusNotFound, errorResponse(err))
					return
				}
				
				c.JSON(http.StatusInternalServerError, errorResponse(err))
				return
			}
			
			// Lấy ảnh chính của Gundam
			primaryImageURL, err := server.dbStore.GetGundamPrimaryImageURL(c.Request.Context(), item.GundamID)
			if err != nil {
				if errors.Is(err, db.ErrRecordNotFound) {
					err = fmt.Errorf("primary image of gundam ID %d not found", item.GundamID)
					c.JSON(http.StatusNotFound, errorResponse(err))
					return
				}
				
				c.JSON(http.StatusInternalServerError, errorResponse(err))
				return
			}
			
			// Lấy ảnh phụ của Gundam
			secondaryImageURLs, err := server.dbStore.GetGundamSecondaryImageURLs(c.Request.Context(), item.GundamID)
			if err != nil {
				if errors.Is(err, db.ErrRecordNotFound) {
					err = fmt.Errorf("secondary images of gundam ID %d not found", item.GundamID)
					c.JSON(http.StatusNotFound, errorResponse(err))
					return
				}
				
				c.JSON(http.StatusInternalServerError, errorResponse(err))
				return
			}
			
			// Lấy thông tin phụ kiện của Gundam
			accessories, err := server.dbStore.GetGundamAccessories(c.Request.Context(), item.GundamID)
			if err != nil {
				if errors.Is(err, db.ErrRecordNotFound) {
					err = fmt.Errorf("accessories of gundam ID %d not found", item.GundamID)
					c.JSON(http.StatusNotFound, errorResponse(err))
					return
				}
				
				c.JSON(http.StatusInternalServerError, errorResponse(err))
				return
			}
			
			// Chuyển đổi phụ kiện sang định dạng DTO
			accessoryDTOs := make([]db.GundamAccessoryDTO, len(accessories))
			for i, accessory := range accessories {
				accessoryDTOs[i] = db.ConvertGundamAccessoryToDTO(accessory)
			}
			
			// Tạo đối tượng GundamDetails
			detail := db.GundamDetails{
				ID:                   item.GundamID,
				OwnerID:              gundam.OwnerID,
				Name:                 gundam.Name,
				Slug:                 gundam.Slug,
				Grade:                grade.DisplayName,
				Series:               gundam.Series,
				PartsTotal:           gundam.PartsTotal,
				Material:             gundam.Material,
				Version:              gundam.Version,
				Quantity:             gundam.Quantity,
				Condition:            string(gundam.Condition),
				ConditionDescription: gundam.ConditionDescription,
				Manufacturer:         gundam.Manufacturer,
				Weight:               gundam.Weight,
				Scale:                string(gundam.Scale),
				Description:          gundam.Description,
				Price:                gundam.Price,
				ReleaseYear:          gundam.ReleaseYear,
				Status:               string(gundam.Status),
				Accessories:          accessoryDTOs,
				PrimaryImageURL:      primaryImageURL,
				SecondaryImageURLs:   secondaryImageURLs,
				CreatedAt:            gundam.CreatedAt,
				UpdatedAt:            gundam.UpdatedAt,
			}
			
			gundamDetails = append(gundamDetails, detail)
		}
		postInfo.ExchangePostItems = gundamDetails
		
		// Đếm số lượng offer cho bài post này
		offerCount, err := server.dbStore.CountExchangeOffers(c.Request.Context(), post.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
		postInfo.OfferCount = offerCount
		
		// Thêm phần lấy offer của người dùng đã đăng nhập nếu có
		if userID != "" {
			offer, err := server.dbStore.GetUserExchangeOfferForPost(c.Request.Context(), db.GetUserExchangeOfferForPostParams{
				PostID:    post.ID,
				OffererID: userID,
			})
			if err != nil {
				if !errors.Is(err, db.ErrRecordNotFound) {
					c.JSON(http.StatusInternalServerError, errorResponse(err))
					return
				}
				// Nếu không tìm thấy offer, thì authenticatedUserOffer sẽ là nil
			} else {
				postInfo.AuthenticatedUserOffer = &offer
				
				// Nếu tìm thấy offer, lấy thêm thông tin chi tiết về các gundam trong offer
				offerItems, err := server.dbStore.ListExchangeOfferItems(c.Request.Context(), offer.ID)
				if err != nil {
					c.JSON(http.StatusInternalServerError, errorResponse(err))
					return
				}
				
				offerGundamDetails := make([]db.GundamDetails, 0, len(offerItems))
				
				for _, item := range offerItems {
					gundam, err := server.dbStore.GetGundamByID(c.Request.Context(), item.GundamID)
					if err != nil {
						if errors.Is(err, db.ErrRecordNotFound) {
							err = fmt.Errorf("gundam ID %d not found", item.GundamID)
							c.JSON(http.StatusNotFound, errorResponse(err))
							return
						}
						
						c.JSON(http.StatusInternalServerError, errorResponse(err))
						return
					}
					
					// Lấy thông tin grade
					grade, err := server.dbStore.GetGradeByID(c.Request.Context(), gundam.GradeID)
					if err != nil {
						if errors.Is(err, db.ErrRecordNotFound) {
							err = fmt.Errorf("grade ID %d not found", gundam.GradeID)
							c.JSON(http.StatusNotFound, errorResponse(err))
							return
						}
						
						c.JSON(http.StatusInternalServerError, errorResponse(err))
						return
					}
					
					// Lấy ảnh chính của Gundam
					primaryImageURL, err := server.dbStore.GetGundamPrimaryImageURL(c.Request.Context(), item.GundamID)
					if err != nil {
						if errors.Is(err, db.ErrRecordNotFound) {
							err = fmt.Errorf("primary image of gundam ID %d not found", item.GundamID)
							c.JSON(http.StatusNotFound, errorResponse(err))
							return
						}
						
						c.JSON(http.StatusInternalServerError, errorResponse(err))
						return
					}
					
					// Lấy ảnh phụ của Gundam
					secondaryImageURLs, err := server.dbStore.GetGundamSecondaryImageURLs(c.Request.Context(), item.GundamID)
					if err != nil {
						if errors.Is(err, db.ErrRecordNotFound) {
							err = fmt.Errorf("secondary images of gundam ID %d not found", item.GundamID)
							c.JSON(http.StatusNotFound, errorResponse(err))
							return
						}
						
						c.JSON(http.StatusInternalServerError, errorResponse(err))
						return
					}
					
					// Lấy thông tin phụ kiện của Gundam
					accessories, err := server.dbStore.GetGundamAccessories(c.Request.Context(), item.GundamID)
					if err != nil {
						if errors.Is(err, db.ErrRecordNotFound) {
							err = fmt.Errorf("accessories of gundam ID %d not found", item.GundamID)
							c.JSON(http.StatusNotFound, errorResponse(err))
							return
						}
						
						c.JSON(http.StatusInternalServerError, errorResponse(err))
						return
					}
					
					// Chuyển đổi phụ kiện sang định dạng DTO
					accessoryDTOs := make([]db.GundamAccessoryDTO, len(accessories))
					for i, accessory := range accessories {
						accessoryDTOs[i] = db.ConvertGundamAccessoryToDTO(accessory)
					}
					
					// Tạo đối tượng GundamDetails
					detail := db.GundamDetails{
						ID:                   item.GundamID,
						OwnerID:              gundam.OwnerID,
						Name:                 gundam.Name,
						Slug:                 gundam.Slug,
						Grade:                grade.DisplayName,
						Series:               gundam.Series,
						PartsTotal:           gundam.PartsTotal,
						Material:             gundam.Material,
						Version:              gundam.Version,
						Quantity:             gundam.Quantity,
						Condition:            string(gundam.Condition),
						ConditionDescription: gundam.ConditionDescription,
						Manufacturer:         gundam.Manufacturer,
						Weight:               gundam.Weight,
						Scale:                string(gundam.Scale),
						Description:          gundam.Description,
						Price:                gundam.Price,
						ReleaseYear:          gundam.ReleaseYear,
						Status:               string(gundam.Status),
						Accessories:          accessoryDTOs,
						PrimaryImageURL:      primaryImageURL,
						SecondaryImageURLs:   secondaryImageURLs,
						CreatedAt:            gundam.CreatedAt,
						UpdatedAt:            gundam.UpdatedAt,
					}
					
					offerGundamDetails = append(offerGundamDetails, detail)
				}
				postInfo.AuthenticatedUserOfferItems = offerGundamDetails
			}
		}
		
		result = append(result, postInfo)
	}
	
	c.JSON(http.StatusOK, result)
}
