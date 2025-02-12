package api

import (
	"errors"
	"net/http"
	
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
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
