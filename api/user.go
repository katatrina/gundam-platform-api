package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"
	
	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/validator"
	"github.com/rs/zerolog/log"
	"google.golang.org/api/idtoken"
	
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
	
	user, err := server.dbStore.CreateUser(context.Background(), arg)
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
	
	user, err := server.dbStore.GetUserByEmail(context.Background(), req.Email)
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
	
	accessToken, accessPayload, err := server.tokenMaker.CreateToken(user.ID, string(user.Role), server.config.AccessTokenDuration)
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
	req := new(loginUserWithGoogleRequest)
	
	if err := ctx.ShouldBindJSON(req); err != nil {
		log.Err(err).Msg("failed to bind json")
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	payload, err := server.googleIDTokenValidator.Validate(ctx, req.IDToken, server.config.GoogleClientID)
	if err != nil {
		log.Err(err).Msg("failed to validate google id token")
		ctx.JSON(http.StatusUnauthorized, errorResponse(err))
		return
	}
	
	// Check identity
	user, err := server.getOrCreateGoogleUser(ctx, payload)
	if err != nil {
		log.Err(err).Msg("failed to get or create google user")
		ctx.JSON(http.StatusInternalServerError, errorResponse(ErrInternalServer))
		return
	}
	
	accessToken, accessPayload, err := server.tokenMaker.CreateToken(user.ID, string(user.Role), server.config.AccessTokenDuration)
	if err != nil {
		log.Err(err).Msg("failed to create access token")
		ctx.JSON(http.StatusInternalServerError, errorResponse(ErrInternalServer))
		return
	}
	
	resp := loginUserResponse{
		AccessToken:          accessToken,
		AccessTokenExpiresAt: accessPayload.ExpiresAt.Time,
		User:                 *user,
	}
	ctx.JSON(http.StatusOK, resp)
}

func (server *Server) getOrCreateGoogleUser(ctx *gin.Context, payload *idtoken.Payload) (*db.User, error) {
	email := payload.Claims["email"].(string)
	user, err := server.dbStore.GetUserByEmail(ctx, email)
	if err == nil {
		return &user, nil
	}
	
	if !errors.Is(err, db.ErrRecordNotFound) {
		log.Err(err).Msg("failed to find user")
		return nil, fmt.Errorf("failed to query user: %w", err)
	}
	
	// User doesn't exist - create new account
	newUser, err := server.dbStore.CreateUserWithGoogleAccount(ctx, db.CreateUserWithGoogleAccountParams{
		ID:            payload.Subject,
		FullName:      payload.Claims["name"].(string),
		Email:         email,
		EmailVerified: payload.Claims["email_verified"].(bool),
		AvatarUrl: pgtype.Text{
			String: payload.Claims["picture"].(string),
			Valid:  true,
		},
	})
	if err != nil {
		log.Err(err).Msg("failed to create user with google account")
		return nil, fmt.Errorf("failed to create user: %w", err)
	}
	
	return &newUser, nil
}

func (server *Server) getUser(ctx *gin.Context) {
	userID := ctx.Param("id")
	
	user, err := server.dbStore.GetUserByID(context.Background(), userID)
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
	FullName *string `json:"full_name"`
}

func (server *Server) updateUser(ctx *gin.Context) {
	userID := ctx.Param("id")
	
	req := new(updateUserRequest)
	
	if err := ctx.ShouldBindJSON(req); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	arg := db.UpdateUserParams{
		UserID: userID,
		FullName: pgtype.Text{
			String: *req.FullName,
			Valid:  req.FullName != nil,
		},
	}
	
	user, err := server.dbStore.UpdateUser(context.Background(), arg)
	if err != nil {
		log.Err(err).Msg("failed to update user")
		ctx.JSON(http.StatusInternalServerError, errorResponse(ErrInternalServer))
		return
	}
	
	ctx.JSON(http.StatusOK, createUserResponse{User: user})
}

type updateAvatarRequest struct {
	Avatar *multipart.FileHeader `form:"avatar" binding:"required"`
}

type updateAvatarResponse struct {
	AvatarURL string `json:"avatar_url"`
}

func (server *Server) updateAvatar(ctx *gin.Context) {
	req := new(updateAvatarRequest)
	
	if err := ctx.ShouldBindWith(&req, binding.FormMultipart); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	// For simplicity, we will skip the validation of the file type and size because frontend already handles it.
	
	userID := ctx.Param("id")
	
	// Get current user to retrieve old avatar URL first
	user, err := server.dbStore.GetUserByID(ctx, userID)
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
	
	// Open and read file
	file, err := req.Avatar.Open()
	if err != nil {
		log.Err(err).Msg("failed to open file")
		ctx.JSON(http.StatusInternalServerError, errorResponse(ErrInternalServer))
		return
	}
	defer file.Close()
	
	fileBytes, err := io.ReadAll(file)
	if err != nil {
		log.Err(err).Msg("failed to read file")
		ctx.JSON(http.StatusInternalServerError, errorResponse(ErrInternalServer))
		return
	}
	
	fileName := fmt.Sprintf("user_%s_%d", userID, time.Now().Unix())
	
	// Upload new avatar to cloudinary
	uploadedFileURL, err := server.fileStore.UploadFile(fileBytes, fileName, FolderAvatars)
	if err != nil {
		log.Err(err).Msg("failed to upload file")
		ctx.JSON(http.StatusInternalServerError, errorResponse(ErrInternalServer))
		return
	}
	
	arg := db.UpdateUserParams{
		AvatarUrl: pgtype.Text{
			String: uploadedFileURL,
			Valid:  true,
		},
		UserID: userID,
	}
	
	user, err = server.dbStore.UpdateUser(ctx, arg)
	if err != nil {
		// Delete newly uploaded avatar if update fails
		if deleteErr := server.fileStore.DeleteFile(fileName, FolderAvatars); deleteErr != nil {
			log.Err(deleteErr).Msg("failed to delete new avatar after update failure")
		}
		
		log.Err(err).Msg("failed to update user avatar")
		ctx.JSON(http.StatusInternalServerError, errorResponse(ErrInternalServer))
		return
	}
	
	ctx.JSON(http.StatusOK, updateAvatarResponse{AvatarURL: user.AvatarUrl.String})
	
	// Optionally, delete old avatar if it exists
}

func (server *Server) getUserByPhoneNumber(ctx *gin.Context) {
	phoneNumber := ctx.Query("phone_number")
	
	user, err := server.dbStore.GetUserByPhoneNumber(context.Background(), pgtype.Text{
		String: phoneNumber,
		Valid:  true,
	})
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("user with phone number %s not found", phoneNumber)
			ctx.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		log.Err(err).Msg("failed to get user by phone number")
		ctx.JSON(http.StatusInternalServerError, errorResponse(ErrInternalServer))
		return
	}
	
	ctx.JSON(http.StatusOK, user)
}
