package api

import (
	"errors"
	"mime/multipart"
	"net/http"
	
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"
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
		ctx.Status(http.StatusInternalServerError)
		return
	}
	
	ctx.JSON(http.StatusOK, grades)
}

type listGundamsRequest struct {
	GradeSlug *string `form:"grade"`
}

type listGundamsResponse []db.ListGundamsWithFiltersRow

func (req *listGundamsRequest) getGradeSlug() string {
	if req == nil || req.GradeSlug == nil {
		return ""
	}
	return *req.GradeSlug
}

//	@Summary		List Gundams
//	@Description	Retrieves a list of Gundams, optionally filtered by grade
//	@Tags			gundams
//	@Produce		json
//	@Param			grade	query		string				false	"Filter by Gundam grade slug"	example(master-grade)
//	@Success		200		{object}	listGundamsResponse	"Successfully retrieved list of Gundams"
//	@Failure		400		"Bad Request - Invalid query parameters"
//	@Failure		500		"Internal Server Error - Failed to retrieve Gundams"
//	@Router			/gundams [get]
func (server *Server) listGundams(ctx *gin.Context) {
	req := new(listGundamsRequest)
	
	if err := ctx.ShouldBindQuery(req); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	gradeSlug := pgtype.Text{
		String: req.getGradeSlug(),
		Valid:  req.GradeSlug != nil,
	}
	
	gundams, err := server.dbStore.ListGundamsWithFilters(ctx, gradeSlug)
	if err != nil {
		log.Error().Err(err).Msg("failed to list gundams")
		ctx.Status(http.StatusInternalServerError)
		return
	}
	
	ctx.JSON(http.StatusOK, listGundamsResponse(gundams))
}

//	@Summary		Get Gundam by Slug
//	@Description	Retrieves a specific Gundam model by its unique slug
//	@Tags			gundams
//	@Produce		json
//	@Param			slug	path		string					true	"Gundam model slug"	example(rx-78-2-gundam)
//	@Success		200		{object}	db.GetGundamBySlugRow	"Successfully retrieved Gundam details"
//	@Failure		404		"Not Found - Gundam with specified slug does not exist"
//	@Failure		500		"Internal Server Error - Failed to retrieve Gundam"
//	@Router			/gundams/{slug} [get]
func (server *Server) getGundamBySlug(ctx *gin.Context) {
	slug := ctx.Param("slug")
	
	gundam, err := server.dbStore.GetGundamBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			ctx.Status(http.StatusNotFound)
			return
		}
		
		log.Error().Err(err).Msg("failed to get gundam by slug")
		ctx.Status(http.StatusInternalServerError)
		return
	}
	
	ctx.JSON(http.StatusOK, gundam)
}

type createGundamRequest struct {
	Name                 string                  `form:"name" binding:"required"`
	GradeID              int64                   `form:"grade_id" binding:"required"`
	Condition            string                  `form:"condition" binding:"required"`
	Manufacturer         string                  `form:"manufacturer" binding:"required"`
	Scale                string                  `form:"scale" binding:"required"`
	Weight               int64                   `form:"weight" binding:"required"`
	Description          string                  `form:"description" binding:"required"`
	Price                int64                   `form:"price" binding:"required"`
	PrimaryImage         *multipart.FileHeader   `form:"primary_image" binding:"required"`
	SecondaryImages      []*multipart.FileHeader `form:"secondary_images" binding:"required"`
	ConditionDescription *string                 `form:"condition_description"`
	Accessories          []db.GundamAccessory    `form:"accessory"`
}

func (req *createGundamRequest) getConditionDescription() string {
	if req.ConditionDescription == nil {
		return ""
	}
	
	return *req.ConditionDescription
}

//	@Summary		Create a new Gundam model
//	@Description	Create a new Gundam model with images and accessories
//	@Tags			gundams
//	@Accept			multipart/form-data
//	@Produce		json
//	@Param			name					formData	string	true	"Gundam name"
//	@Param			grade_id				formData	integer	true	"Gundam grade ID"
//	@Param			condition				formData	string	true	"Condition of the Gundam"	Enums(new, open box, second hand)
//	@Param			manufacturer			formData	string	true	"Manufacturer name"
//	@Param			scale					formData	string	true	"Gundam scale"	Enums(1/144, 1/100, 1/60)
//	@Param			weight					formData	integer	true	"Weight in grams"
//	@Param			description				formData	string	true	"Detailed description"
//	@Param			price					formData	integer	true	"Price in VND"
//	@Param			primary_image			formData	file	true	"Primary image of the Gundam"
//	@Param			secondary_images		formData	file	true	"Secondary images of the Gundam"
//	@Param			condition_description	formData	string	false	"Additional details about condition"
//	@Param			accessory				formData	string	false	"Accessory as JSON object. Add multiple accessories by repeating this field with different values."
//	@Security		accessToken
//	@Success		200	"message: Gundam created successfully"
//	@Failure		400	"error details"
//	@Failure		401	"unauthorized"
//	@Failure		500	"internal server error"
//	@Router			/users/:id/gundams [post]
func (server *Server) createGundam(ctx *gin.Context) {
	req := new(createGundamRequest)
	
	if err := ctx.ShouldBind(req); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	userID := ctx.Param("id")
	ownerID := ctx.MustGet(authorizationPayloadKey).(*token.Payload).Subject
	if userID != ownerID {
		ctx.JSON(http.StatusUnauthorized, gin.H{"message": "cannot create gundam for another user"})
		return
	}
	
	arg := db.CreateGundamTxParams{
		OwnerID:   ownerID,
		Name:      req.Name,
		Slug:      util.GenerateRandomSlug(req.Name),
		GradeID:   req.GradeID,
		Condition: db.GundamCondition(req.Condition),
		ConditionDescription: pgtype.Text{
			String: req.getConditionDescription(),
			Valid:  req.ConditionDescription != nil,
		},
		Manufacturer:     req.Manufacturer,
		Weight:           req.Weight,
		Scale:            db.GundamScale(req.Scale),
		Description:      req.Description,
		Price:            req.Price,
		Accessories:      req.Accessories,
		PrimaryImage:     req.PrimaryImage,
		SecondaryImages:  req.SecondaryImages,
		UploadImagesFunc: server.uploadFileToCloudinary,
	}
	
	err := server.dbStore.CreateGundamTx(ctx, arg)
	if err != nil {
		log.Error().Err(err).Msg("failed to create gundam")
		ctx.Status(http.StatusInternalServerError)
		return
	}
	
	ctx.JSON(http.StatusOK, gin.H{"message": "Gundam created successfully"})
}
