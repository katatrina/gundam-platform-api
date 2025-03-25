package api

// import (
// 	"errors"
// 	"net/http"
//
// 	"github.com/gin-gonic/gin"
// 	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
// 	"github.com/katatrina/gundam-BE/internal/token"
// )
//
// // @Summary		Retrieve user wallet
// // @Description	Get the wallet information of the authenticated user
// // @Tags			users
// // @Produce		json
// // @Security		accessToken
// // @Success		200	{object}	db.Wallet	"Successfully retrieved wallet"
// // @Failure		404	"Wallet not found"
// // @Failure		500	"Internal server error"
// // @Router			/users/:id/wallet [get]
// func (server *Server) getUserWallet(ctx *gin.Context) {
// 	userID := ctx.MustGet(authorizationPayloadKey).(*token.Payload).Subject
//
// 	wallet, err := server.dbStore.GetWalletByUserID(ctx, userID)
// 	if err != nil {
// 		if errors.Is(err, db.ErrRecordNotFound) {
// 			ctx.JSON(http.StatusNotFound, errorResponse(err))
// 			return
// 		}
//
// 		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
// 		return
// 	}
//
// 	ctx.JSON(http.StatusOK, wallet)
// }
//
// func (server *Server) updateUserWallet(ctx *gin.Context) {
// 	userID := ctx.MustGet(authorizationPayloadKey).(*token.Payload).Subject
//
// 	var req db.UpdateWalletParams
// 	if err := ctx.ShouldBindJSON(&req); err != nil {
// 		ctx.JSON(http.StatusBadRequest, errorResponse(err))
// 		return
// 	}
//
// 	req.UserID = userID
//
// 	wallet, err := server.dbStore.UpdateWallet(ctx, req)
// 	if err != nil {
// 		if errors.Is(err, db.ErrRecordNotFound) {
// 			ctx.JSON(http.StatusNotFound, errorResponse(err))
// 			return
// 		}
//
// 		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
// 		return
// 	}
//
// 	ctx.JSON(http.StatusOK, wallet)
// }
