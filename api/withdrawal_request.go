package api

import (
	"errors"
	"fmt"
	"net/http"
	
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	db "github.com/katatrina/gundam-BE/internal/db/sqlc"
	"github.com/katatrina/gundam-BE/internal/token"
	"github.com/katatrina/gundam-BE/internal/util"
)

type createWithdrawalRequestBody struct {
	BankAccountID uuid.UUID `json:"bank_account_id"`
	Amount        int64     `json:"amount"`
}

func (req *createWithdrawalRequestBody) validate() error {
	if req.BankAccountID == uuid.Nil {
		return fmt.Errorf("bank_account_id is required")
	}
	
	if req.Amount <= 0 {
		return fmt.Errorf("amount must be greater than zero")
	}
	
	if req.Amount < 50_000 || req.Amount > 5_000_000 {
		return fmt.Errorf("amount must be between 50.000 and 5.000.000")
	}
	
	return nil
}

//	@Summary		Create withdrawal request
//	@Description	Create a new withdrawal request to transfer money from user's wallet to their bank account
//	@Description
//	@Description	**Quy định nghiệp vụ:**
//	@Description	- Người dùng phải có đủ số dư trong ví (số tiền sẽ bị trừ ngay lập tức)
//	@Description	- Tài khoản ngân hàng phải thuộc về người dùng đã xác thực
//	@Description	- Số tiền rút phải trong khoảng từ 50,000 VNĐ đến 5,000,000 VNĐ mỗi lần
//	@Description	- Yêu cầu rút tiền được xử lý thủ công bởi moderator trong vòng 24 giờ (ngày làm việc)
//	@Description	- Sau khi tạo, yêu cầu rút tiền không thể hủy bởi người dùng
//	@Description
//	@Description	**Quy trình xử lý:**
//	@Description	1. Hệ thống kiểm tra yêu cầu và số dư ví
//	@Description	2. Tiền được trừ ngay lập tức khỏi ví người dùng
//	@Description	3. Yêu cầu rút tiền được tạo với trạng thái "đang xử lý"
//	@Description	4. ModeratorID xem xét và xử lý yêu cầu thủ công
//	@Description	5. Trạng thái cập nhật thành "hoàn thành" sau khi chuyển tiền
//	@Description
//	@Description	**Các trường hợp lỗi:**
//	@Description	- 400: Dữ liệu yêu cầu không hợp lệ hoặc không đủ số dư
//	@Description	- 404: Không tìm thấy tài khoản ngân hàng hoặc không thuộc về người dùng
//	@Description	- 422: Vi phạm quy tắc nghiệp vụ (giới hạn số tiền, v.v.)
//	@Tags			wallet
//	@Accept			json
//	@Produce		json
//	@Security		accessToken
//	@Param			request	body		createWithdrawalRequestBody	true	"Withdrawal request details"
//	@Success		201		{object}	db.WithdrawalRequest		"Withdrawal request created successfully"
//	@Router			/users/me/wallet/withdrawal-requests [post]
func (server *Server) createWithdrawalRequest(c *gin.Context) {
	authPayload := c.MustGet(authorizationPayloadKey).(*token.Payload)
	userID := authPayload.Subject
	
	var req createWithdrawalRequestBody
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	// Validate business logic
	
	if err := req.validate(); err != nil {
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	wallet, err := server.dbStore.GetWalletByUserID(c, userID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("user wallet ID %s not found", userID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	if wallet.Balance < req.Amount {
		err = fmt.Errorf("insufficient balance: current balance is %s, requested amount is %s", util.FormatMoney(wallet.Balance), util.FormatMoney(req.Amount))
		c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
		return
	}
	
	_, err = server.dbStore.GetUserBankAccount(c, db.GetUserBankAccountParams{
		ID:     req.BankAccountID,
		UserID: userID,
	})
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("bank account ID %s not found for user ID %s", req.BankAccountID, userID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	// Excute transaction to create withdrawal request
	request, err := server.dbStore.CreateWithdrawalRequestTx(c, db.CreateWithdrawalRequestTxParams{
		UserID:        userID,
		BankAccountID: req.BankAccountID,
		Amount:        req.Amount,
	})
	if err != nil {
		err = fmt.Errorf("failed to create withdrawal request for user ID %s: %w", userID, err)
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	c.JSON(http.StatusCreated, request)
}

//	@Summary		List user withdrawal requests
//	@Description	Retrieve all withdrawal requests for the authenticated user
//	@Tags			wallet
//	@Produce		json
//	@Security		accessToken
//	@Param			status	query	string						false	"Filter by status"	Enums(pending, approved, completed, rejected, canceled)
//	@Success		200		{array}	db.WithdrawalRequestDetails	"List of withdrawal requests"
//	@Router			/users/me/wallet/withdrawal-requests [get]
func (server *Server) listUserWithdrawalRequests(c *gin.Context) {
	authPayload := c.MustGet(authorizationPayloadKey).(*token.Payload)
	userID := authPayload.Subject
	
	status := c.Query("status")
	if status != "" {
		if err := db.IsValidWithdrawalRequestStatus(status); err != nil {
			c.JSON(http.StatusBadRequest, errorResponse(err))
			return
		}
	}
	
	requests, err := server.dbStore.ListUserWithdrawalRequests(c, db.ListUserWithdrawalRequestsParams{
		UserID: userID,
		Status: db.NullWithdrawalRequestStatus{
			WithdrawalRequestStatus: db.WithdrawalRequestStatus(status),
			Valid:                   status != "",
		},
	})
	if err != nil {
		err = fmt.Errorf("failed to list withdrawal requests for user ID %s: %w", userID, err)
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	resp := make([]db.WithdrawalRequestDetails, 0, len(requests))
	for _, req := range requests {
		resp = append(resp, db.NewWithdrawalRequestDetails(req.WithdrawalRequest, req.UserBankAccount))
	}
	
	c.JSON(http.StatusOK, resp)
}

//	@Summary		Cancel withdrawal request
//	@Description	Cancel a pending withdrawal request and refund the amount back to user's wallet
//	@Description
//	@Description	**Quy định nghiệp vụ:**
//	@Description	- Chỉ có thể hủy những yêu cầu rút tiền đang ở trạng thái chờ xử lý
//	@Description	- Chỉ người tạo yêu cầu mới có thể hủy yêu cầu của mình
//	@Description	- Số tiền sẽ được hoàn trả ngay lập tức vào ví của người dùng
//	@Description
//	@Description	**Các trường hợp lỗi:**
//	@Description	- 400: ID yêu cầu không đúng định dạng
//	@Description	- 403: Yêu cầu không thuộc về người dùng
//	@Description	- 404: Không tìm thấy yêu cầu
//	@Description	- 422: Không thể hủy yêu cầu (không ở trạng thái chờ xử lý)
//	@Tags			wallet
//	@Produce		json
//	@Security		accessToken
//	@Param			requestID	path		string	true	"Withdrawal request ID"
//	@Success		200			{object}	db.WithdrawalRequestDetails
//	@Router			/users/me/wallet/withdrawal-requests/{requestID}/cancel [patch]
func (server *Server) cancelWithdrawalRequest(c *gin.Context) {
	authPayload := c.MustGet(authorizationPayloadKey).(*token.Payload)
	userID := authPayload.Subject
	
	requestID, err := uuid.Parse(c.Param("requestID"))
	if err != nil {
		err = fmt.Errorf("invalid request ID: %s", c.Param("requestID"))
		c.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}
	
	request, err := server.dbStore.GetWithdrawalRequest(c, requestID)
	if err != nil {
		if errors.Is(err, db.ErrRecordNotFound) {
			err = fmt.Errorf("withdrawal request ID %s not found", requestID)
			c.JSON(http.StatusNotFound, errorResponse(err))
			return
		}
		
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	if request.WithdrawalRequest.UserID != userID {
		err = fmt.Errorf("withdrawal request ID %s does not belong to user ID %s", requestID, userID)
		c.JSON(http.StatusForbidden, errorResponse(err))
		return
	}
	
	if request.WithdrawalRequest.Status != db.WithdrawalRequestStatusPending {
		err = fmt.Errorf("withdrawal request ID %s cannot be canceled, current status: %s", requestID, request.WithdrawalRequest.Status)
		c.JSON(http.StatusUnprocessableEntity, errorResponse(err))
		return
	}
	
	// Execute transaction to cancel withdrawal request
	updatedRequest, err := server.dbStore.CancelWithdrawalRequestTx(c, db.CancelWithdrawalRequestTxParams{
		WithdrawalRequest: request.WithdrawalRequest,
	})
	if err != nil {
		err = fmt.Errorf("failed to cancel withdrawal request ID %s for user ID %s: %w", requestID, userID, err)
		c.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	
	c.JSON(http.StatusOK, db.NewWithdrawalRequestDetails(updatedRequest, request.UserBankAccount))
}
