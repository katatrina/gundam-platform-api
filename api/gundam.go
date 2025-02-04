package api

import (
	"net/http"
	
	"github.com/gin-gonic/gin"
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

func (server *Server) listGundams(ctx *gin.Context) {
	gundams, err := server.dbStore.ListGundams(ctx)
	if err != nil {
		log.Error().Err(err).Msg("failed to list gundams")
		ctx.JSON(http.StatusInternalServerError, errorResponse(ErrInternalServer))
		return
	}
	
	ctx.JSON(http.StatusOK, gundams)
}
