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

// CreateUserRequest represents the input for creating a new user
type createUserRequest struct {
	FullName string `json:"full_name" binding:"required"`
	Email    string `json:"email" binding:"required" `
	Password string `json:"password" binding:"required"`
}

// CreateUser godoc
//	@Summary		Create a new user
//	@Description	Create a new user with email and password
//	@Tags			authentication
//	@Accept			json
//	@Produce		json
//	@Param			request	body		createUserRequest	true	"User creation request"
//	@Success		201		{object}	db.User				"Successfully created user"
//	@Failure		400		"Invalid request body"
//	@Failure		422		"Validation error"
//	@Failure		409		"Email already exists"
//	@Failure		500		"Internal server error"
//	@Router			/users [post]
func (server *Server) createUser(ctx *gin.Context) {
	req := new(createUserRequest)
	
	if err := ctx.ShouldBindJSON(req); err != nil {
		ctx.Status(http.StatusBadRequest)
		return
	}
	
	violations := validateCreateUserRequest(req)
	if violations != nil {
		ctx.JSON(http.StatusUnprocessableEntity, failedValidationError(violations))
		return
	}
	
	hashedPassword, err := util.HashPassword(req.Password)
	if err != nil {
		ctx.Status(http.StatusInternalServerError)
		return
	}
	
	arg := db.CreateUserParams{
		FullName: req.FullName,
		HashedPassword: pgtype.Text{
			String: hashedPassword,
			Valid:  true,
		},
		Email:         req.Email,
		EmailVerified: true,
		Role:          db.UserRoleMember,
	}
	
	user, err := server.dbStore.CreateUserTx(context.Background(), arg)
	if err != nil {
		if pgErr := db.ErrorDescription(err); pgErr != nil {
			switch {
			case pgErr.Code == db.UniqueViolationCode && pgErr.ConstraintName == db.UniqueEmailConstraint:
				err = errors.New("email already exists")
				ctx.JSON(http.StatusConflict, errorResponse(err))
				return
			}
			
			log.Err(err).Msg("failed to create user")
			ctx.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
	}
	
	ctx.JSON(http.StatusCreated, user)
}

// validateCreateUserRequest performs validation on the create user request
func validateCreateUserRequest(req *createUserRequest) (violations []*FieldViolation) {
	if err := validator.ValidateFullName(req.FullName); err != nil {
		violations = append(violations, fieldViolation("full_name", err))
	}
	
	if err := validator.ValidateEmail(req.Email); err != nil {
		violations = append(violations, fieldViolation("email", err))
	}
	
	if err := validator.ValidatePassword(req.Password); err != nil {
		violations = append(violations, fieldViolation("password", err))
	}
	
	return violations
}

type loginUserRequest struct {
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type loginUserResponse struct {
	User                 db.User   `json:"user"`
	AccessToken          string    `json:"access_token"`
	AccessTokenExpiresAt time.Time `json:"access_token_expires_at"`
}

//	@Summary		Login user
//	@Description	Authenticate a user and return access token
//	@Tags			authentication
//	@Accept			json
//	@Produce		json
//	@Param			request	body		loginUserRequest	true	"Login credentials"
//	@Success		200		{object}	loginUserResponse
//	@Failure		400		"Invalid request parameters"
//	@Failure		401		"Incorrect password"
//	@Failure		404		"Email not found"
//	@Failure		500		"Internal server error"
//	@Router			/auth/login [post]
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
		ctx.Status(http.StatusInternalServerError)
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
		ctx.Status(http.StatusInternalServerError)
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

//	@Summary		Login or register a user with Google account
//	@Description	Authenticate a user using Google ID token. If the user doesn't exist, a new user will be created.
//	@Tags			authentication
//	@Accept			json
//	@Produce		json
//	@Param			request	body		loginUserWithGoogleRequest	true	"Google ID Token"
//	@Success		200		{object}	loginUserResponse			"Successfully logged in"
//	@Failure		400		"Invalid request body"
//	@Failure		401		"Invalid Google ID token"
//	@Failure		500		"Internal server error"
//	@Router			/auth/google-login [post]
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
		ctx.Status(http.StatusInternalServerError)
		return
	}
	
	accessToken, accessPayload, err := server.tokenMaker.CreateToken(user.ID, string(user.Role), server.config.AccessTokenDuration)
	if err != nil {
		log.Err(err).Msg("failed to create access token")
		ctx.Status(http.StatusInternalServerError)
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
	
	err = server.dbStore.CreateWallet(ctx, user.ID)
	if err != nil {
		log.Err(err).Msg("failed to create wallet")
		return nil, fmt.Errorf("failed to create wallet: %w", err)
	}
	
	return &newUser, nil
}

//	@Summary		Retrieve a user by ID
//	@Description	Get detailed information about a specific user
//	@Tags			users
//	@Produce		json
//	@Param			id	path		string	true	"User ID"
//	@Success		200	{object}	db.User	"Successfully retrieved user"
//	@Failure		404	"User not found"
//	@Failure		500	"Internal server error"
//	@Router			/users/{id} [get]
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
		ctx.Status(http.StatusInternalServerError)
		return
	}
	
	ctx.JSON(http.StatusOK, user)
}

// UpdateUserRequest represents the input for updating a user
type updateUserRequest struct {
	FullName *string `json:"full_name" `
}

//	@Summary		Update a user's information
//	@Description	Update specific user details by user ID
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string				true	"User ID"
//	@Param			request	body		updateUserRequest	true	"User update request"
//	@Success		200		{object}	db.User				"Successfully updated user"
//	@Failure		400		"Invalid request body"
//	@Failure		500		"Internal server error"
//	@Router			/users/{id} [put]
func (server *Server) updateUser(ctx *gin.Context) {
	userID := ctx.Param("id")
	
	req := new(updateUserRequest)
	
	if err := ctx.ShouldBindJSON(req); err != nil {
		ctx.Status(http.StatusBadRequest)
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
		ctx.Status(http.StatusInternalServerError)
		return
	}
	
	ctx.JSON(http.StatusOK, user)
}

// UpdateAvatarRequest represents the input for updating a user's avatar
type updateAvatarRequest struct {
	Avatar *multipart.FileHeader `form:"avatar" binding:"required"`
}

//	@Summary		Update user avatar
//	@Description	Upload and update a user's profile avatar
//	@Tags			users
//	@Accept			multipart/form-data
//	@Produce		json
//	@Param			id		path		string					true	"User ID"
//	@Param			avatar	formData	file					true	"Avatar image file"
//	@Success		200		{object}	updateAvatarResponse	"Successfully updated avatar"
//	@Failure		400		"Invalid request"
//	@Failure		404		"User not found"
//	@Failure		500		"Internal server error"
//	@Router			/users/{id}/avatar [patch]
func (server *Server) updateAvatar(ctx *gin.Context) {
	req := new(updateAvatarRequest)
	
	if err := ctx.ShouldBindWith(&req, binding.FormMultipart); err != nil {
		ctx.Status(http.StatusBadRequest)
		return
	}
	
	// For simplicity, we will skip the validation of the file type and size because frontend already handles it.
	
	userID := ctx.Param("id")
	
	// Get current user to retrieve old avatar URL first
	user, err := server.dbStore.GetUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			ctx.Status(http.StatusNotFound)
			return
		}
		
		log.Err(err).Msg("failed to get user")
		ctx.Status(http.StatusInternalServerError)
		return
	}
	
	// Open and read file
	file, err := req.Avatar.Open()
	if err != nil {
		log.Err(err).Msg("failed to open file")
		ctx.Status(http.StatusInternalServerError)
		return
	}
	defer file.Close()
	
	fileBytes, err := io.ReadAll(file)
	if err != nil {
		log.Err(err).Msg("failed to read file")
		ctx.Status(http.StatusInternalServerError)
		return
	}
	
	fileName := fmt.Sprintf("user_%s_%d", userID, time.Now().Unix())
	
	// Upload new avatar to cloudinary
	uploadedFileURL, err := server.fileStore.UploadFile(fileBytes, fileName, util.FolderAvatars)
	if err != nil {
		log.Err(err).Msg("failed to upload file")
		ctx.Status(http.StatusInternalServerError)
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
		if deleteErr := server.fileStore.DeleteFile(fileName, util.FolderAvatars); deleteErr != nil {
			log.Err(deleteErr).Msg("failed to delete new avatar after update failure")
		}
		
		log.Err(err).Msg("failed to update user avatar")
		ctx.Status(http.StatusInternalServerError)
		return
	}
	
	resp := updateAvatarResponse{
		AvatarURL: user.AvatarUrl.String,
	}
	ctx.JSON(http.StatusOK, resp)
	
	// TODO: delete old avatar after successful update
}

// UpdateAvatarResponse represents the response after updating a user's avatar
type updateAvatarResponse struct {
	AvatarURL string `json:"avatar_url" binding:"required"`
}

//	@Summary		Retrieve a user by phone_number number
//	@Description	Get user details using a phone_number number as a query parameter
//	@Tags			users
//	@Produce		json
//	@Param			phone_number	query		string	true	"Phone Number"
//	@Success		200				{object}	db.User	"Successfully retrieved user"
//	@Failure		404				"User not found"
//	@Failure		500				"Internal server error"
//	@Router			/users/by-phone_number [get]
func (server *Server) getUserByPhoneNumber(ctx *gin.Context) {
	phoneNumber := ctx.Query("phone_number")
	
	user, err := server.dbStore.GetUserByPhoneNumber(context.Background(), pgtype.Text{
		String: phoneNumber,
		Valid:  true,
	})
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			ctx.Status(http.StatusNotFound)
			return
		}
		
		log.Err(err).Msg("failed to get user by phone_number number")
		ctx.Status(http.StatusInternalServerError)
		return
	}
	
	ctx.JSON(http.StatusOK, user)
}
