package api

import (
	"net/http"
	
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/rs/zerolog/log"
)

func (server *Server) listGundamGrades(ctx *gin.Context) {
	grades, err := server.dbStore.ListGundamGrades(ctx)
	if err != nil {
		log.Error().Err(err).Msg("failed to list gundam grades")
		ctx.JSON(http.StatusInternalServerError, errorResponse(ErrInternalServer))
		return
	}
	
	ctx.JSON(http.StatusOK, grades)
}

type listGundamsRequest struct {
	GradeSlug *string `form:"grade"`
}

type listGundamsResponse struct {
	Gundams []struct {
		db.ListGundamsWithFiltersRow
		Images []db.GundamImage `json:"images"`
	} `json:"data"`
}

func (req *listGundamsRequest) getGradeSlug() string {
	if req == nil || req.GradeSlug == nil {
		return ""
	}
	
	return *req.GradeSlug
}

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
		ctx.JSON(http.StatusInternalServerError, errorResponse(ErrInternalServer))
		return
	}
	
	// Initialize the response with the correct capacity
	resp := &listGundamsResponse{
		Gundams: make([]struct {
			db.ListGundamsWithFiltersRow
			Images []db.GundamImage `json:"images"`
		}, 0, len(gundams)),
	}
	
	// Use a single loop to build the response
	for _, gundam := range gundams {
		images, err := server.dbStore.ListGundamImages(ctx, gundam.ID)
		if err != nil {
			log.Error().Err(err).Msg("failed to list gundam images")
			ctx.JSON(http.StatusInternalServerError, errorResponse(ErrInternalServer))
			return
		}
		
		resp.Gundams = append(resp.Gundams, struct {
			db.ListGundamsWithFiltersRow
			Images []db.GundamImage `json:"images"`
		}{
			ListGundamsWithFiltersRow: gundam,
			Images:                    images,
		})
	}
	
	// Return the response struct, not the gundams slice
	ctx.JSON(http.StatusOK, resp)
}
