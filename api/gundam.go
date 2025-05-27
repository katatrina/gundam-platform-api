package api

import (
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"strconv"
	
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/token"
	"github.com/katatrina/gundam-BE/internal/util"
	"github.com/rs/zerolog/log"
)

//	@Summary		List Gundam Grades
//	@Description	Retrieves a list of all available Gundam model grades
//	@Tags			gundams
//	@Produce		json
//	@Success		200	{array}	db.GundamGrade	"Successfully retrieved list of Gundam grades"
//	@Failure		500	"Internal Server Error - Failed to retrieve Gundam grades"
//	@Router			/grades [get]
func (server *Server) listGundamGrades(ctx *gin.Context) {
	grades, err := server.dbStore.ListGundamGrades(ctx)
	if err != nil {
		log.Error().Err(err).Msg("failed to retrieve all gundam grades")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	ctx.JSON(http.StatusOK, grades)
}

type listGundamsRequest struct {
	Name      *string `form:"name"`
	GradeSlug *string `form:"grade"`
	Status    *string `form:"status"`
}

func (r *listGundamsRequest) validate() error {
	if r.Status != nil {
		return db.IsValidGundamStatus(*r.Status)
	}
	
	return nil
}

//	@Summary		List Gundams
//	@Description	Retrieves a list of selling Gundams, optionally filtered by grade
//	@Tags			gundams
//	@Produce		json
//	@Param			name	query	string				false	"Filter by Gundam name"			example(YR-04 Fire Lord)
//	@Param			grade	query	string				false	"Filter by Gundam grade slug"	example(master-grade)
//	@Param			status	query	string				false	"Filter by Gundam status"		Enums(in store, published, processing, pending auction approval, auctioning)
//	@Success		200		array	db.GundamDetails	"Successfully retrieved list of Gundams"
//	@Failure		400		"Bad Request - Invalid query parameters"
//	@Failure		500		"Internal Server Error - Failed to retrieve Gundams"
//	@Router			/gundams [get]
func (server *Server) listGundams(ctx *gin.Context) {
	req := new(listGundamsRequest)
	
	if err := ctx.ShouldBindQuery(req); err != nil {
		log.Error().Err(err).Msg("failed to bind query parameters")
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	// Validate the status parameter
	if err := req.validate(); err != nil {
		log.Error().Err(err).Msg("invalid status")
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	arg := db.ListGundamsWithFiltersParams{
		GradeSlug: req.GradeSlug,
		Status:    req.Status,
		Name:      req.Name,
	}
	
	result, err := server.dbStore.ListGundamsWithFilters(ctx, arg)
	if err != nil {
		log.Error().Err(err).Msg("failed to list result")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	resp := make([]db.GundamDetails, len(result))
	
	// Map the result to the response struct
	for i, row := range result {
		primaryImageURL, err := server.dbStore.GetGundamPrimaryImageURL(ctx, row.GundamID)
		if err != nil {
			log.Error().Err(err).Msg("failed to get primary image")
			ctx.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
		
		secondaryImageURLs, err := server.dbStore.GetGundamSecondaryImageURLs(ctx, row.GundamID)
		if err != nil {
			log.Error().Err(err).Msg("failed to get secondary images")
			ctx.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
		
		accessories, err := server.dbStore.GetGundamAccessories(ctx, row.GundamID)
		if err != nil {
			log.Error().Err(err).Msg("failed to get gundam accessories")
			ctx.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
		
		// Map the accessories to the response struct
		accessoryDTOs := make([]db.GundamAccessoryDTO, len(accessories))
		for i, accessory := range accessories {
			accessoryDTOs[i] = db.ConvertGundamAccessoryToDTO(accessory)
		}
		
		resp[i] = db.GundamDetails{
			ID:                   row.GundamID,
			OwnerID:              row.OwnerID,
			Name:                 row.Name,
			Slug:                 row.Slug,
			Grade:                row.Grade,
			Series:               row.Series,
			PartsTotal:           row.PartsTotal,
			Material:             row.Material,
			Version:              row.Version,
			Quantity:             row.Quantity,
			Condition:            string(row.Condition),
			ConditionDescription: row.ConditionDescription,
			Manufacturer:         row.Manufacturer,
			Weight:               row.Weight,
			Scale:                string(row.Scale),
			Description:          row.Description,
			Price:                row.Price,
			ReleaseYear:          row.ReleaseYear,
			Status:               string(row.Status),
			Accessories:          accessoryDTOs,
			PrimaryImageURL:      primaryImageURL,
			SecondaryImageURLs:   secondaryImageURLs,
			CreatedAt:            row.CreatedAt,
			UpdatedAt:            row.UpdatedAt,
		}
	}
	
	ctx.JSON(http.StatusOK, resp)
}

type getGundamBySlugQuery struct {
	Status *string `form:"status"`
}

func (req *getGundamBySlugQuery) validate() error {
	if req.Status != nil {
		return db.IsValidGundamStatus(*req.Status)
	}
	
	return nil
}

//	@Summary		Get Gundam by Slug
//	@Description	Retrieves a specific Gundam model by its unique slug
//	@Tags			gundams
//	@Produce		json
//	@Param			slug	path		string				true	"Gundam model slug"			example(rx-78-2-gundam)
//	@Param			status	query		string				false	"Filter by Gundam status"	Enums(in store, published, processing, pending auction approval, auctioning)
//	@Success		200		{object}	db.GundamDetails	"Successfully retrieved Gundam details"
//	@Failure		404		"Not Found - Gundam with specified slug does not exist"
//	@Failure		500		"Internal Server Error - Failed to retrieve Gundam"
//	@Router			/gundams/by-slug/{slug} [get]
func (server *Server) getGundamBySlug(ctx *gin.Context) {
	slug := ctx.Param("slug")
	
	var query getGundamBySlugQuery
	if err := ctx.ShouldBindQuery(&query); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	if err := query.validate(); err != nil {
		log.Error().Err(err).Msg("invalid status")
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	var resp db.GundamDetails
	
	arg := db.GetGundamBySlugParams{
		Slug:   slug,
		Status: query.Status,
	}
	
	row, err := server.dbStore.GetGundamBySlug(ctx, arg)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("gundam with slug %s not found", slug)
			ctx.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Error().Err(err).Msg("failed to get gundam by slug")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	resp.ID = row.GundamID
	resp.OwnerID = row.OwnerID
	resp.Name = row.Name
	resp.Slug = row.Slug
	resp.Grade = row.Grade
	resp.Series = row.Series
	resp.PartsTotal = row.PartsTotal
	resp.Material = row.Material
	resp.Version = row.Version
	resp.Quantity = row.Quantity
	resp.Condition = string(row.Condition)
	resp.ConditionDescription = row.ConditionDescription
	resp.Manufacturer = row.Manufacturer
	resp.Weight = row.Weight
	resp.Scale = string(row.Scale)
	resp.Description = row.Description
	resp.Price = row.Price
	resp.ReleaseYear = row.ReleaseYear
	resp.Status = string(row.Status)
	resp.CreatedAt = row.CreatedAt
	resp.UpdatedAt = row.UpdatedAt
	
	primaryImageURL, err := server.dbStore.GetGundamPrimaryImageURL(ctx, row.GundamID)
	if err != nil {
		log.Error().Err(err).Msg("failed to get primary image")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	resp.PrimaryImageURL = primaryImageURL
	
	secondaryImageURLs, err := server.dbStore.GetGundamSecondaryImageURLs(ctx, row.GundamID)
	if err != nil {
		log.Error().Err(err).Msg("failed to get secondary images")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	resp.SecondaryImageURLs = secondaryImageURLs
	
	accessories, err := server.dbStore.GetGundamAccessories(ctx, row.GundamID)
	if err != nil {
		log.Error().Err(err).Msg("failed to get gundam accessories")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Map the accessories to the response struct
	accessoryDTOs := make([]db.GundamAccessoryDTO, len(accessories))
	for i, accessory := range accessories {
		accessoryDTOs[i] = db.ConvertGundamAccessoryToDTO(accessory)
	}
	resp.Accessories = accessoryDTOs
	
	ctx.JSON(http.StatusOK, resp)
}

type createGundamRequest struct {
	Name                 string                  `form:"name" binding:"required"`
	GradeID              int64                   `form:"grade_id" binding:"required"`
	Series               string                  `form:"series" binding:"required"`
	PartsTotal           int64                   `form:"parts_total" binding:"required"`
	Material             string                  `form:"material" binding:"required"`
	Version              string                  `form:"version" binding:"required"`
	Condition            string                  `form:"condition" binding:"required"`
	Manufacturer         string                  `form:"manufacturer" binding:"required"`
	Scale                string                  `form:"scale" binding:"required"`
	Weight               int64                   `form:"weight" binding:"required"`
	Description          string                  `form:"description" binding:"required"`
	Price                *int64                  `form:"price"`
	ReleaseYear          *int64                  `form:"release_year"`
	PrimaryImage         *multipart.FileHeader   `form:"primary_image" binding:"required"`
	SecondaryImages      []*multipart.FileHeader `form:"secondary_images" binding:"required"`
	ConditionDescription *string                 `form:"condition_description"`
	Accessories          []db.GundamAccessoryDTO `form:"accessory"`
}

//	@Summary		Create a new Gundam model
//	@Description	Create a new Gundam model with images and accessories
//	@Tags			users
//	@Accept			multipart/form-data
//	@Produce		json
//	@Param			id						path		string	true	"User ID"
//	@Param			name					formData	string	true	"Gundam name"
//	@Param			grade_id				formData	integer	true	"Gundam grade ID"
//	@Param			series					formData	string	true	"Gundam series name"
//	@Param			parts_total				formData	integer	true	"Total number of parts"
//	@Param			material				formData	string	true	"Gundam material"
//	@Param			version					formData	string	true	"Gundam version"
//	@Param			condition				formData	string	true	"Condition of the Gundam"	Enums(new, open box, used)
//	@Param			manufacturer			formData	string	true	"Manufacturer name"
//	@Param			scale					formData	string	true	"Gundam scale"	Enums(1/144, 1/100, 1/60)
//	@Param			weight					formData	integer	true	"Weight in grams"
//	@Param			description				formData	string	true	"Detailed description"
//	@Param			price					formData	integer	false	"Price in VND"
//	@Param			release_year			formData	integer	false	"Release year"
//	@Param			primary_image			formData	file	true	"Primary image of the Gundam"
//	@Param			secondary_images		formData	file	true	"Secondary images of the Gundam"
//	@Param			condition_description	formData	string	false	"Additional details about condition"
//	@Param			accessory				formData	string	false	"Accessory as JSON object. Add multiple accessories by repeating this field with different values."
//	@Security		accessToken
//	@Success		201	{object}	db.GundamDetails	"Successfully created Gundam"
//	@Failure		400	"Bad Request - Invalid input data"
//	@Failure		404	"Not Found - User with specified ID does not exist"
//	@Failure		403	"Forbidden - User is not authorized to create Gundam for this user"
//	@Failure		500	"Internal Server Error - Failed to create Gundam"
//	@Router			/users/:id/gundams [post]
func (server *Server) createGundam(ctx *gin.Context) {
	authPayload := ctx.MustGet(authorizationPayloadKey).(*token.Payload)
	authenticatedUserID := authPayload.Subject
	userID := ctx.Param("id")
	
	_, err := server.dbStore.GetUserByID(ctx.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("user ID %s not found", userID)
			ctx.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Error().Err(err).Msg("failed to get user by user ID")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	if authenticatedUserID != userID {
		err = fmt.Errorf("authenticated user ID %s is not authorized to create gundam for user ID %s", authenticatedUserID, userID)
		ctx.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	req := new(createGundamRequest)
	if err := ctx.ShouldBindWith(req, binding.FormMultipart); err != nil {
		log.Error().Err(err).Msg("failed to bind request")
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	arg := db.CreateGundamTxParams{
		OwnerID:              userID,
		Name:                 req.Name,
		Slug:                 util.GenerateRandomSlug(req.Name),
		GradeID:              req.GradeID,
		Series:               req.Series,
		PartsTotal:           req.PartsTotal,
		Material:             req.Material,
		Version:              req.Version,
		Quantity:             1, // Default quantity is 1
		Condition:            db.GundamCondition(req.Condition),
		ConditionDescription: req.ConditionDescription,
		Manufacturer:         req.Manufacturer,
		Weight:               req.Weight,
		Scale:                db.GundamScale(req.Scale),
		Description:          req.Description,
		Price:                req.Price,
		ReleaseYear:          req.ReleaseYear,
		Accessories:          req.Accessories,
		PrimaryImage:         req.PrimaryImage,
		SecondaryImages:      req.SecondaryImages,
		UploadImagesFunc:     server.uploadFileToCloudinary,
	}
	
	result, err := server.dbStore.CreateGundamTx(ctx, arg)
	if err != nil {
		log.Error().Err(err).Msg("failed to create gundam")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	ctx.JSON(http.StatusCreated, result)
}

type listGundamsByUserRequest struct {
	Name *string `form:"name"`
}

//	@Summary		List all gundams for a specific user
//	@Description	Get all gundams that belong to the specified user ID
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Param			id		path	string	true	"User ID"
//	@Param			name	query	string	false	"Gundam name to filter by"
//	@Security		accessToken
//	@Success		200	array	db.GundamDetails	"Successfully retrieved list of gundams"
//	@Router			/users/:id/gundams [get]
func (server *Server) listGundamsByUser(ctx *gin.Context) {
	authPayload := ctx.MustGet(authorizationPayloadKey).(*token.Payload)
	authenticatedUserID := authPayload.Subject
	userID := ctx.Param("id")
	
	_, err := server.dbStore.GetUserByID(ctx.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("user ID %s not found", userID)
			ctx.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Error().Err(err).Msg("failed to get user by ID")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	if authenticatedUserID != userID {
		err = fmt.Errorf("authenticated user ID %s is not authorized to view gundams for user ID %s", authenticatedUserID, userID)
		ctx.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	req := new(listGundamsByUserRequest)
	if err := ctx.ShouldBindQuery(req); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	arg := db.ListGundamsByUserIDParams{
		OwnerID: userID,
		Name:    req.Name,
	}
	
	gundams, err := server.dbStore.ListGundamsByUserID(ctx, arg)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list gundams by seller")
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	resp := make([]db.GundamDetails, len(gundams))
	
	for i, gundam := range gundams {
		primaryImageURL, err := server.dbStore.GetGundamPrimaryImageURL(ctx, gundam.ID)
		if err != nil {
			if errors.Is(err, db.ErrRecordNotFound) {
				err = fmt.Errorf("gundam ID %d primary image not found", gundam.ID)
				ctx.JSON(http.StatusNotFound, errorResponse(err))
				return
			}
			
			log.Error().Err(err).Msg("failed to get primary image")
			ctx.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
		
		secondaryImageURLs, err := server.dbStore.GetGundamSecondaryImageURLs(ctx, gundam.ID)
		if err != nil {
			log.Error().Err(err).Msg("failed to get secondary images")
			ctx.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
		
		accessories, err := server.dbStore.GetGundamAccessories(ctx, gundam.ID)
		if err != nil {
			log.Error().Err(err).Msg("failed to get gundam accessories")
			ctx.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
		
		// Map the accessories to the response struct
		accessoryDTOs := make([]db.GundamAccessoryDTO, len(accessories))
		for i, accessory := range accessories {
			accessoryDTOs[i] = db.ConvertGundamAccessoryToDTO(accessory)
		}
		
		resp[i] = db.GundamDetails{
			ID:                   gundam.ID,
			OwnerID:              gundam.OwnerID,
			Name:                 gundam.Name,
			Slug:                 gundam.Slug,
			Grade:                gundam.Grade,
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
	}
	
	ctx.JSON(http.StatusOK, resp)
}

type updateGundamBasisInfoRequest struct {
	Name                 *string `json:"name" binding:"omitempty,min=1,max=255"`
	GradeID              *int64  `json:"grade_id" binding:"omitempty,gt=0"`
	Series               *string `json:"series" binding:"omitempty,min=1,max=255"`
	PartsTotal           *int64  `json:"parts_total" binding:"omitempty,gt=0"`
	Material             *string `json:"material" binding:"omitempty,min=1,max=255"`
	Version              *string `json:"version" binding:"omitempty,min=1,max=255"`
	Condition            *string `json:"condition" binding:"omitempty,oneof=new 'open box' used"`
	ConditionDescription *string `json:"condition_description" binding:"omitempty,max=1000"`
	Manufacturer         *string `json:"manufacturer" binding:"omitempty,min=1,max=255"`
	Weight               *int64  `json:"weight" binding:"omitempty,gt=0"`
	Scale                *string `json:"scale" binding:"omitempty,oneof=1/144 1/100 1/60 1/48"`
	Description          *string `json:"description" binding:"omitempty,min=1"`
	Price                *int64  `json:"price" binding:"omitempty,gte=0"`
	ReleaseYear          *int64  `json:"release_year" binding:"omitempty,gt=1900,lt=2100"`
}

func (req *updateGundamBasisInfoRequest) getCondition() string {
	if req.Condition == nil {
		return ""
	}
	
	return *req.Condition
}

func (req *updateGundamBasisInfoRequest) getScale() string {
	if req.Scale == nil {
		return ""
	}
	
	return *req.Scale
}

//	@Summary		Update Gundam basis info
//	@Description	Update the basic information of a Gundam model
//	@Tags			gundams
//	@Accept			json
//	@Produce		json
//	@Param			id						path	string	true	"User ID"
//	@Param			gundamID				path	string	true	"Gundam ID"
//	@Param			name					body	string	false	"Gundam name"
//	@Param			grade_id				body	integer	false	"Gundam grade ID"
//	@Param			series					body	string	false	"Gundam series name"
//	@Param			parts_total				body	integer	false	"Total number of parts"
//	@Param			material				body	string	false	"Gundam material"
//	@Param			version					body	string	false	"Gundam version"
//	@Param			condition				body	string	false	"Condition of the Gundam"	Enums(new, open box, used)
//	@Param			condition_description	body	string	false	"Additional details about condition"
//	@Param			manufacturer			body	string	false	"Manufacturer name"
//	@Param			weight					body	integer	false	"Weight in grams"
//	@Param			scale					body	string	false	"Gundam scale"	Enums(1/144, 1/100, 1/60, 1/48)
//	@Param			description				body	string	false	"Detailed description"
//	@Param			price					body	integer	false	"Price in VND"
//	@Param			release_year			body	integer	false	"Release year"
//	@Security		accessToken
//	@Success		200	{object}	db.GundamDetails	"Successfully updated Gundam"
//	@Router			/users/:id/gundams/:gundamID [patch]
func (server *Server) updateGundamBasisInfo(c *gin.Context) {
	authPayload := c.MustGet(authorizationPayloadKey).(*token.Payload)
	authenticatedUserID := authPayload.Subject
	userID := c.Param("id")
	
	_, err := server.dbStore.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("user ID %s not found", userID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Error().Err(err).Msg("failed to get user by user ID")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	if authenticatedUserID != userID {
		err = fmt.Errorf("authenticated user ID %s is not authorized to update gundam for user ID %s", authenticatedUserID, userID)
		c.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	gundamID, err := strconv.ParseInt(c.Param("gundamID"), 10, 64)
	if err != nil {
		err = fmt.Errorf("invalid gundam ID %s", c.Param("gundamID"))
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	gundam, err := server.dbStore.GetGundamByID(c.Request.Context(), gundamID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("gundam ID %s not found", gundamID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Error().Err(err).Msg("failed to get gundam by gundam ID")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	if gundam.OwnerID != userID {
		err = fmt.Errorf("user ID %s is not the owner of gundam ID %d", userID, gundamID)
		c.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	if gundam.Status != db.GundamStatusInstore {
		err = fmt.Errorf("gundam ID %d is not in store", gundamID)
		c.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	var req updateGundamBasisInfoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	arg := db.UpdateGundamParams{
		ID:         gundamID,
		Name:       req.Name,
		GradeID:    req.GradeID,
		Series:     req.Series,
		PartsTotal: req.PartsTotal,
		Material:   req.Material,
		Version:    req.Version,
		Condition: db.NullGundamCondition{
			GundamCondition: db.GundamCondition(req.getCondition()),
			Valid:           req.Condition != nil,
		},
		ConditionDescription: req.ConditionDescription,
		Manufacturer:         req.Manufacturer,
		Weight:               req.Weight,
		Scale: db.NullGundamScale{
			GundamScale: db.GundamScale(req.getScale()),
			Valid:       req.Scale != nil,
		},
		Description: req.Description,
		Price:       req.Price,
		ReleaseYear: req.ReleaseYear,
	}
	
	err = server.dbStore.UpdateGundam(c.Request.Context(), arg)
	if err != nil {
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Return the updated Gundam details
	updatedGundam, err := server.dbStore.GetGundamDetailsByID(c.Request.Context(), nil, gundamID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("gundam ID %s not found", gundamID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Error().Err(err).Msg("failed to get updated gundam details")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	c.JSON(http.StatusOK, updatedGundam)
}

//	@Summary		Get Gundam details
//	@Description	Retrieves detailed information about a specific Gundam model
//	@Tags			gundams
//	@Produce		json
//	@Param			id			path		string				true	"User ID"
//	@Param			gundamID	path		string				true	"Gundam ID"
//	@Success		200			{object}	db.GundamDetails	"Successfully retrieved Gundam details"
//	@Router			/users/:id/gundams/:gundamID [get]
//	@Security		accessToken
func (server *Server) getUserGundamDetails(c *gin.Context) {
	authPayload := c.MustGet(authorizationPayloadKey).(*token.Payload)
	authenticatedUserID := authPayload.Subject
	userID := c.Param("id")
	
	_, err := server.dbStore.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("user ID %s not found", userID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Error().Err(err).Msg("failed to get user by user ID")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	if authenticatedUserID != userID {
		err = fmt.Errorf("authenticated user ID %s is not authorized to view gundam details for user ID %s", authenticatedUserID, userID)
		c.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	gundamID, err := strconv.ParseInt(c.Param("gundamID"), 10, 64)
	if err != nil {
		err = fmt.Errorf("invalid gundam ID %s", c.Param("gundamID"))
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	gundamDetails, err := server.dbStore.GetGundamDetailsByID(c.Request.Context(), nil, gundamID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("gundam ID %d not found", gundamID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Error().Err(err).Msg("failed to get gundam details")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	if gundamDetails.OwnerID != userID {
		err = fmt.Errorf("user ID %s is not the owner of gundam ID %d", userID, gundamID)
		c.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	c.JSON(http.StatusOK, gundamDetails)
}

//	@Summary		Hard delete Gundam
//	@Description	Hard delete a Gundam model by its ID
//	@Tags			gundams
//	@Produce		json
//	@Param			id			path	string	true	"User ID"
//	@Param			gundamID	path	string	true	"Gundam ID"
//	@Success		204			"Successfully deleted Gundam"
//	@Router			/users/:id/gundams/:gundamID [delete]
//	@Security		accessToken
func (server *Server) hardDeleteGundam(c *gin.Context) {
	authPayload := c.MustGet(authorizationPayloadKey).(*token.Payload)
	authenticatedUserID := authPayload.Subject
	userID := c.Param("id")
	
	_, err := server.dbStore.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("user ID %s not found", userID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Error().Err(err).Msg("failed to get user by user ID")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	if authenticatedUserID != userID {
		err = fmt.Errorf("authenticated user ID %s is not authorized to delete gundam for user ID %s", authenticatedUserID, userID)
		c.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	gundamID, err := strconv.ParseInt(c.Param("gundamID"), 10, 64)
	if err != nil {
		err = fmt.Errorf("invalid gundam ID %s", c.Param("gundamID"))
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	gundam, err := server.dbStore.GetGundamByID(c.Request.Context(), gundamID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("gundam ID %d not found", gundamID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Error().Err(err).Msg("failed to get gundam by gundam ID")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	if gundam.OwnerID != userID {
		err = fmt.Errorf("user ID %s is the owner of gundam ID %d", userID, gundamID)
		c.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	if gundam.Status != db.GundamStatusInstore {
		err = fmt.Errorf("gundam ID %d is not in store", gundamID)
		c.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	err = server.dbStore.DeleteGundam(c.Request.Context(), db.DeleteGundamParams{
		ID:      gundamID,
		OwnerID: userID,
	})
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("gundam ID %d not found", gundamID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		// TODO: Thêm xử lý cho lỗi do ràng buộc khóa ngoại (foreign key constraint), đặc biệt từ các bảng exchange_post_items và exchange_offer_items.
		
		log.Error().Err(err).Msg("failed to delete gundam")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	c.JSON(http.StatusNoContent, nil)
}

//	@Summary		Update Gundam accessories
//	@Description	Update the accessories of a Gundam model
//	@Tags			gundams
//	@Accept			json
//	@Produce		json
//	@Param			id			path	string					true	"User ID"
//	@Param			gundamID	path	string					true	"Gundam ID"	
//	@Param			request		body	[]db.GundamAccessoryDTO	true	"Array of Gundam accessories"
//	@Security		accessToken
//	@Success		200	{object}	db.GundamDetails	"Successfully updated Gundam details"
//	@Router			/users/:id/gundams/:gundamID/accessories [put]
func (server *Server) updateGundamAccessories(c *gin.Context) {
	authPayload := c.MustGet(authorizationPayloadKey).(*token.Payload)
	authenticatedUserID := authPayload.Subject
	userID := c.Param("id")
	
	_, err := server.dbStore.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("user ID %s not found", userID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Error().Err(err).Msg("failed to get user by user ID")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	if authenticatedUserID != userID {
		err = fmt.Errorf("authenticated user ID %s is not authorized to update gundam for user ID %s", authenticatedUserID, userID)
		c.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	gundamID, err := strconv.ParseInt(c.Param("gundamID"), 10, 64)
	if err != nil {
		err = fmt.Errorf("invalid gundam ID %s", c.Param("gundamID"))
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	gundam, err := server.dbStore.GetGundamByID(c.Request.Context(), gundamID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("gundam ID %d not found", gundamID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Error().Err(err).Msg("failed to get gundam by gundam ID")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	if gundam.OwnerID != userID {
		err = fmt.Errorf("user ID %s is not the owner of gundam ID %d", userID, gundamID)
		c.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	if gundam.Status != db.GundamStatusInstore {
		err = fmt.Errorf("gundam ID %d is not in store", gundamID)
		c.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	accessories := make([]db.GundamAccessoryDTO, 0)
	if err = c.ShouldBindJSON(&accessories); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	arg := db.UpdateGundamAccessoriesParams{
		GundamID:    gundamID,
		Accessories: accessories,
	}
	
	err = server.dbStore.UpdateGundamAccessoriesTx(c.Request.Context(), arg)
	if err != nil {
		log.Error().Err(err).Msg("failed to update gundam accessories")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Return the updated Gundam details
	updatedGundam, err := server.dbStore.GetGundamDetailsByID(c.Request.Context(), nil, gundamID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("gundam ID %d not found", gundamID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Error().Err(err).Msg("failed to get updated gundam details")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	c.JSON(http.StatusOK, updatedGundam)
}

type updateGundamPrimaryImageRequest struct {
	PrimaryImage *multipart.FileHeader `form:"primary_image" binding:"required"`
}

//	@Summary		Update Gundam primary image
//	@Description	Update the primary image of a Gundam model
//	@Tags			gundams
//	@Accept			multipart/form-data
//	@Produce		json
//	@Param			id				path		string	true	"User ID"
//	@Param			gundamID		path		string	true	"Gundam ID"
//	@Param			primary_image	formData	file	true	"New primary image of the Gundam"
//	@Security		accessToken
//	@Success		200	{object}	db.GundamDetails	"Successfully updated Gundam details"
//	@Failure		400	"Bad Request - Invalid input data"
//	@Failure		404	"Not Found - User or Gundam with specified ID does not exist"
//	@Failure		403	"Forbidden - User is not authorized to update Gundam for this user"
//	@Failure		500	"Internal Server Error - Failed to update Gundam primary image"
//	@Router			/users/:id/gundams/:gundamID/primary-image [patch]
func (server *Server) updateGundamPrimaryImage(c *gin.Context) {
	authPayload := c.MustGet(authorizationPayloadKey).(*token.Payload)
	authenticatedUserID := authPayload.Subject
	userID := c.Param("id")
	
	_, err := server.dbStore.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("user ID %s not found", userID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Error().Err(err).Msg("failed to get user by user ID")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	if authenticatedUserID != userID {
		err = fmt.Errorf("authenticated user ID %s is not authorized to update gundam for user ID %s", authenticatedUserID, userID)
		c.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	gundamID, err := strconv.ParseInt(c.Param("gundamID"), 10, 64)
	if err != nil {
		err = fmt.Errorf("invalid gundam ID %s", c.Param("gundamID"))
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	gundam, err := server.dbStore.GetGundamByID(c.Request.Context(), gundamID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("gundam ID %d not found", gundamID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Error().Err(err).Msg("failed to get gundam by gundam ID")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	if gundam.OwnerID != userID {
		err = fmt.Errorf("user ID %s is not the owner of gundam ID %d", userID, gundamID)
		c.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	if gundam.Status != db.GundamStatusInstore {
		err = fmt.Errorf("gundam ID %d is not in store", gundamID)
		c.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	var req updateGundamPrimaryImageRequest
	if err := c.ShouldBindWith(&req, binding.FormMultipart); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	// Upload the new primary image to Cloudinary
	uploadedFileURLs, err := server.uploadFileToCloudinary("gundam", gundam.Slug, util.FolderGundams, req.PrimaryImage)
	if err != nil {
		log.Error().Err(err).Msg("failed to upload primary image")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	if len(uploadedFileURLs) == 0 {
		err = fmt.Errorf("no primary image URL returned from Cloudinary")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	currentPrimaryImageURL, err := server.dbStore.GetGundamPrimaryImageURL(c.Request.Context(), gundamID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("gundam ID %d primary image not found", gundamID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Error().Err(err).Msg("failed to get primary image")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	currentPrimaryImagePublicID, err := util.ExtractPublicIDFromURL(currentPrimaryImageURL)
	if err != nil {
		log.Error().Err(err).Msg("failed to extract public ID from URL")
	}
	
	primaryImageURL := uploadedFileURLs[0]
	err = server.dbStore.UpdateGundamPrimaryImage(c.Request.Context(), db.UpdateGundamPrimaryImageParams{
		GundamID: gundamID,
		URL:      primaryImageURL,
	})
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("gundam ID %d not found", gundamID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Error().Err(err).Msg("failed to update gundam primary image")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Delete the old primary image from Cloudinary
	err = server.fileStore.DeleteFile(currentPrimaryImagePublicID, "")
	if err != nil {
		log.Error().Err(err).Msg("failed to delete old primary image from Cloudinary")
	}
	
	// Return the updated Gundam details
	updatedGundam, err := server.dbStore.GetGundamDetailsByID(c.Request.Context(), nil, gundamID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("gundam ID %d not found", gundamID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Error().Err(err).Msg("failed to get updated gundam details")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	c.JSON(http.StatusOK, updatedGundam)
}

type addGundamImagesRequest struct {
	Images []*multipart.FileHeader `form:"images" binding:"required"`
}

//	@Summary		Add secondary images to Gundam
//	@Description	Add secondary images to a Gundam model
//	@Tags			gundams
//	@Accept			multipart/form-data
//	@Produce		json
//	@Security		accessToken
//	@Param			id			path		string	true	"User ID"
//	@Param			gundamID	path		string	true	"Gundam ID"
//	@Param			images		formData	file	true	"Array of secondary images"
//	@Success		200			"Successfully added secondary images"
//	@Router			/users/:id/gundams/:gundamID/images [post]
func (server *Server) addGundamSecondaryImages(c *gin.Context) {
	authPayload := c.MustGet(authorizationPayloadKey).(*token.Payload)
	authenticatedUserID := authPayload.Subject
	userID := c.Param("id")
	
	_, err := server.dbStore.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("user ID %s not found", userID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Error().Err(err).Msg("failed to get user by user ID")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	if authenticatedUserID != userID {
		err = fmt.Errorf("authenticated user ID %s is not authorized to update gundam for user ID %s", authenticatedUserID, userID)
		c.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	gundamID, err := strconv.ParseInt(c.Param("gundamID"), 10, 64)
	if err != nil {
		err = fmt.Errorf("invalid gundam ID %s", c.Param("gundamID"))
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	gundam, err := server.dbStore.GetGundamByID(c.Request.Context(), gundamID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("gundam ID %d not found", gundamID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Error().Err(err).Msg("failed to get gundam by gundam ID")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	if gundam.OwnerID != userID {
		err = fmt.Errorf("user ID %s is not the owner of gundam ID %d", userID, gundamID)
		c.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	if gundam.Status != db.GundamStatusInstore {
		err = fmt.Errorf("gundam ID %d is not in store", gundamID)
		c.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	var req addGundamImagesRequest
	if err := c.ShouldBindWith(&req, binding.FormMultipart); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	result, err := server.dbStore.AddGundamSecondaryImagesTx(c.Request.Context(), db.AddGundamSecondaryImagesTxParams{
		Gundam:           gundam,
		Images:           req.Images,
		UploadImagesFunc: server.uploadFileToCloudinary,
	})
	if err != nil {
		log.Error().Err(err).Msg("failed to add gundam secondary images")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"message":    "Secondary images added successfully",
		"image_urls": result.ImageURLs,
	})
}

type deleteGundamSecondaryImageRequest struct {
	ImageURL string `json:"image_url" binding:"required"`
}

//	@Summary		Delete secondary image from Gundam
//	@Description	Delete a secondary image from a Gundam model
//	@Tags			gundams
//	@Accept			json
//	@Produce		json
//	@Param			id			path	string	true	"User ID"
//	@Param			gundamID	path	string	true	"Gundam ID"
//	@Param			image_url	body	string	true	"URL of the secondary image to delete"
//	@Security		accessToken
//	@Success		204	"Successfully deleted secondary image"
//	@Router			/users/:id/gundams/:gundamID/images [delete]
func (server *Server) deleteGundamSecondaryImage(c *gin.Context) {
	authPayload := c.MustGet(authorizationPayloadKey).(*token.Payload)
	authenticatedUserID := authPayload.Subject
	userID := c.Param("id")
	
	_, err := server.dbStore.GetUserByID(c.Request.Context(), userID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("user ID %s not found", userID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Error().Err(err).Msg("failed to get user by user ID")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	if authenticatedUserID != userID {
		err = fmt.Errorf("authenticated user ID %s is not authorized to update gundam for user ID %s", authenticatedUserID, userID)
		c.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	gundamID, err := strconv.ParseInt(c.Param("gundamID"), 10, 64)
	if err != nil {
		err = fmt.Errorf("invalid gundam ID %s", c.Param("gundamID"))
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	gundam, err := server.dbStore.GetGundamByID(c.Request.Context(), gundamID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("gundam ID %d not found", gundamID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Error().Err(err).Msg("failed to get gundam by gundam ID")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	if gundam.OwnerID != userID {
		err = fmt.Errorf("user ID %s is not the owner of gundam ID %d", userID, gundamID)
		c.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	if gundam.Status != db.GundamStatusInstore {
		err = fmt.Errorf("gundam ID %d is not in store", gundamID)
		c.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	var req deleteGundamSecondaryImageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	gundamImage, err := server.dbStore.GetImageByURL(c.Request.Context(), db.GetImageByURLParams{
		URL:      req.ImageURL,
		GundamID: gundamID,
	})
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("gundam ID %d image URL not found", gundamID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Error().Err(err).Msg("failed to get gundam image by URL")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	if gundamImage.IsPrimary {
		err = fmt.Errorf("cannot delete primary image")
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	publicID, err := util.ExtractPublicIDFromURL(req.ImageURL)
	if err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(fmt.Errorf("invalid image URL: %w", err)))
		return
	}
	
	err = server.dbStore.DeleteGundamSecondaryImageTx(c.Request.Context(), db.DeleteGundamSecondaryImageTxParams{
		GundamImage:     gundamImage,
		PublicID:        publicID,
		DeleteImageFunc: server.fileStore.DeleteFile,
	})
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("gundam ID %d image URL not found", gundamID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Error().Err(err).Msg("failed to delete gundam secondary image")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	c.JSON(http.StatusNoContent, nil)
}

//	@Summary		Get Gundam details
//	@Description	Retrieves detailed information about a specific Gundam model
//	@Tags			gundams
//	@Produce		json
//	@Param			gundamID	path		string				true	"Gundam ID"
//	@Success		200			{object}	db.GundamDetails	"Successfully retrieved Gundam details"
//	@Router			/gundams/:gundamID [get]
func (server *Server) getGundamDetails(c *gin.Context) {
	gundamIDStr := c.Param("gundamID")
	gundamID, err := strconv.ParseInt(gundamIDStr, 10, 64)
	if err != nil {
		err = fmt.Errorf("invalid gundam ID %s", gundamIDStr)
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	gundam, err := server.dbStore.GetGundamDetailsByID(c.Request.Context(), nil, gundamID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("gundam ID %d not found", gundamID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Error().Err(err).Msg("failed to get gundam details")
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	c.JSON(http.StatusOK, gundam)
}
