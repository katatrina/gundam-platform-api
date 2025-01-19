package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
	
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/validator"
	"github.com/rs/zerolog/log"
	
	"github.com/katatrina/gundam-BE/internal/util"
)

type createUserRequest struct {
	Password string `json:"password"`
	Email    string `json:"email"`
}

type createUserResponse struct {
	User db.User `json:"user"`
}

func validateCreateUserRequest(req *createUserRequest) (violations []*FieldViolation) {
	if err := validator.ValidateEmail(req.Email); err != nil {
		violations = append(violations, fieldViolation("email", err))
	}
	
	return violations
}

func (server *Server) createUser(ctx *gin.Context) {
	req := new(createUserRequest)
	
	if err := ctx.ShouldBindJSON(req); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	violations := validateCreateUserRequest(req)
	if violations != nil {
		ctx.JSON(http.StatusUnprocessableEntity, failedValidationError(violations))
		return
	}
	
	hashedPassword, err := util.HashPassword(req.Password)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(fmt.Errorf("failed to hash password: %w", err)))
		return
	}
	
	arg := db.CreateUserParams{
		HashedPassword: pgtype.Text{
			String: hashedPassword,
			Valid:  true,
		},
		Email:         req.Email,
		EmailVerified: false,
	}
	
	user, err := server.store.CreateUser(context.Background(), arg)
	if err != nil {
		errCode, constraintName := db.ErrorDescription(err)
		switch {
		case errCode == db.UniqueViolationCode && constraintName == db.UniqueEmailConstraint:
			err = fmt.Errorf("email %s already exists", req.Email)
			ctx.JSON(http.StatusConflict, errorResponse(err))
			return
		}
		
		log.Err(err).Msg("failed to create user")
		ctx.JSON(http.StatusInternalServerError, errorResponse(ErrInternalServer))
		return
	}
	
	ctx.JSON(http.StatusOK, createUserResponse{User: user})
}

type loginUserRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginUserResponse struct {
	User                 db.User   `json:"user"`
	AccessToken          string    `json:"access_token"`
	AccessTokenExpiresAt time.Time `json:"access_token_expires_at"`
}

func (server *Server) loginUser(ctx *gin.Context) {
	req := new(loginUserRequest)
	
	if err := ctx.ShouldBindJSON(req); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	user, err := server.store.GetUserByEmail(context.Background(), req.Email)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = errors.New("email not found")
			ctx.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Err(err).Msg("failed to find user")
		ctx.JSON(http.StatusInternalServerError, errorResponse(ErrInternalServer))
		return
	}
	
	err = util.CheckPassword(req.Password, user.HashedPassword.String)
	if err != nil {
		err = errors.New("incorrect password")
		ctx.JSON(http.StatusUnauthorized, errorResponse(err))
		return
	}
	
	accessToken, accessPayload, err := server.tokenMaker.CreateToken(user.ID, user.Role, server.config.AccessTokenDuration)
	if err != nil {
		log.Err(err).Msg("failed to create access token")
		ctx.JSON(http.StatusInternalServerError, errorResponse(ErrInternalServer))
		return
	}
	
	resp := loginUserResponse{
		AccessToken:          accessToken,
		AccessTokenExpiresAt: accessPayload.ExpiresAt.Time,
		User:                 user,
	}
	ctx.JSON(http.StatusOK, resp)
}

type loginUserWithGoogleRequest struct {
	IDToken string `json:"id_token" binding:"required"`
}

func (server *Server) loginUserWithGoogle(ctx *gin.Context) {
	log.Info().Msg("Received Google login request")
	req := new(loginUserWithGoogleRequest)
	
	if err := ctx.ShouldBindJSON(req); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	log.Info().Msgf("Received ID token length: %d", len(req.IDToken))
	
	payload, err := server.googleIDTokenValidator.Validate(ctx, req.IDToken, server.config.GoogleClientID)
	if err != nil {
		log.Err(err).Msg("failed to validate google id token")
		ctx.JSON(http.StatusUnauthorized, errorResponse(err))
		return
	}
	
	// Check identity
	user, err := server.store.GetUserByEmail(context.Background(), payload.Claims["email"].(string))
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			// Create a new user
			arg := db.CreateUserWithGoogleAccountParams{
				ID:            payload.Subject,
				Name:          payload.Claims["name"].(string),
				Email:         payload.Claims["email"].(string),
				EmailVerified: payload.Claims["email_verified"].(bool),
				Avatar: pgtype.Text{
					String: payload.Claims["picture"].(string),
					Valid:  true,
				},
			}
			
			user, err = server.store.CreateUserWithGoogleAccount(context.Background(), arg)
			if err != nil {
				log.Err(err).Msg("failed to create user with google account")
				ctx.JSON(http.StatusInternalServerError, errorResponse(ErrInternalServer))
				return
			}
			
			return
		}
		
		log.Err(err).Msg("failed to find user")
		ctx.JSON(http.StatusInternalServerError, errorResponse(ErrInternalServer))
		return
	}
	
	accessToken, accessPayload, err := server.tokenMaker.CreateToken(user.ID, user.Role, server.config.AccessTokenDuration)
	if err != nil {
		log.Err(err).Msg("failed to create access token")
		ctx.JSON(http.StatusInternalServerError, errorResponse(ErrInternalServer))
		return
	}
	
	resp := loginUserResponse{
		AccessToken:          accessToken,
		AccessTokenExpiresAt: accessPayload.ExpiresAt.Time,
		User:                 user,
	}
	ctx.JSON(http.StatusOK, resp)
}

func (server *Server) getUser(ctx *gin.Context) {
	userID := ctx.Param("id")
	
	user, err := server.store.GetUserByID(context.Background(), userID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("user %s not found", userID)
			ctx.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Err(err).Msg("failed to get user")
		ctx.JSON(http.StatusInternalServerError, errorResponse(ErrInternalServer))
		return
	}
	
	ctx.JSON(http.StatusOK, user)
}

type updateUserRequest struct {
	Name *string `json:"name"`
}

func (server *Server) updateUser(ctx *gin.Context) {
	userID := ctx.Param("id")
	
	req := new(updateUserRequest)
	
	if err := ctx.ShouldBindJSON(req); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	arg := db.UpdateUserParams{
		ID: userID,
		Name: pgtype.Text{
			String: *req.Name,
			Valid:  req.Name != nil,
		},
	}
	
	user, err := server.store.UpdateUser(context.Background(), arg)
	if err != nil {
		log.Err(err).Msg("failed to update user")
		ctx.JSON(http.StatusInternalServerError, errorResponse(ErrInternalServer))
		return
	}
	
	ctx.JSON(http.StatusOK, createUserResponse{User: user})
}
