package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"saruman/internal/dto"
	apperrors "saruman/internal/errors"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

type ReserveAndAddUseCase interface {
	ReserveItems(ctx context.Context, orderID uint, companyID int, items []dto.ReservationItem) (*dto.ReservationResult, error)
}

type ReserveAndAddController struct {
	useCase ReserveAndAddUseCase
	logger  *zap.Logger
}

func NewReserveAndAddController(useCase ReserveAndAddUseCase, logger *zap.Logger) *ReserveAndAddController {
	return &ReserveAndAddController{
		useCase: useCase,
		logger:  logger,
	}
}

func (c *ReserveAndAddController) ReserveAndAdd(w http.ResponseWriter, r *http.Request) {
	traceID := uuid.New().String()
	logger := c.logger.With(zap.String("traceId", traceID))

	// Parse orderId from path
	orderIDStr := chi.URLParam(r, "orderId")
	orderID, err := strconv.ParseUint(orderIDStr, 10, 64)
	if err != nil {
		logger.Warn("invalid orderId in path", zap.Error(err))
		c.writeValidationError(w, traceID, "invalid orderId", apperrors.ValidationDetail{
			Field:   "orderId",
			Message: "orderId must be a positive integer",
		})
		return
	}

	// Decode request body
	var req dto.ReserveAndAddRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.Warn("invalid JSON body", zap.Error(err))
		c.writeValidationError(w, traceID, "invalid JSON body", apperrors.ValidationDetail{
			Field:   "body",
			Message: "request body must be valid JSON",
		})
		return
	}

	// Validate request
	if validationErr := c.validateReserveAndAddRequest(uint(orderID), req); validationErr != nil {
		ve, _ := apperrors.IsValidationError(validationErr)
		c.writeValidationError(w, traceID, ve.Message, ve.Details...)
		return
	}

	// Map request items to ReservationItem
	items := make([]dto.ReservationItem, len(req.Items))
	for i, item := range req.Items {
		items[i] = dto.ReservationItem{
			ProductID: item.ProductID,
			Quantity:  item.Quantity,
			Price:     item.Price,
		}
	}

	// Call use case
	result, err := c.useCase.ReserveItems(r.Context(), uint(orderID), req.CompanyID, items)
	if err != nil {
		c.handleUseCaseError(w, traceID, uint(orderID), err, logger)
		return
	}

	// Map result to response
	c.writeReserveAndAddResponse(w, traceID, result)
}

func (c *ReserveAndAddController) validateReserveAndAddRequest(orderID uint, req dto.ReserveAndAddRequest) error {
	var details []apperrors.ValidationDetail

	// Validate orderId
	if orderID <= 0 {
		details = append(details, apperrors.ValidationDetail{
			Field:   "orderId",
			Message: "orderId must be a positive integer",
		})
	}

	// Validate companyId
	if req.CompanyID <= 0 {
		msg := "companyId must be a positive integer"
		if req.CompanyID == 0 {
			msg = "companyId is required"
		}
		details = append(details, apperrors.ValidationDetail{
			Field:   "companyId",
			Message: msg,
		})
	}

	// Validate items is not empty
	if len(req.Items) == 0 {
		details = append(details, apperrors.ValidationDetail{
			Field:   "items",
			Message: "items must not be empty",
		})
	}

	// Validate items length <= 100
	if len(req.Items) > 100 {
		details = append(details, apperrors.ValidationDetail{
			Field:   "items",
			Message: "items exceeds maximum of 100",
		})
	}

	// Track product IDs for duplicate detection
	productIDMap := make(map[int]bool)

	// Validate each item
	for idx, item := range req.Items {
		if item.ProductID <= 0 {
			details = append(details, apperrors.ValidationDetail{
				Field:   "items[" + strconv.Itoa(idx) + "].productId",
				Message: "each productId must be a positive integer",
			})
		}

		if productIDMap[item.ProductID] {
			details = append(details, apperrors.ValidationDetail{
				Field:   "items[" + strconv.Itoa(idx) + "].productId",
				Message: "productId must not be duplicated",
			})
		}
		productIDMap[item.ProductID] = true

		if item.Quantity < 1 || item.Quantity > 10000 {
			details = append(details, apperrors.ValidationDetail{
				Field:   "items[" + strconv.Itoa(idx) + "].quantity",
				Message: "quantity must be between 1 and 10000",
			})
		}

		if item.Price < 0 {
			details = append(details, apperrors.ValidationDetail{
				Field:   "items[" + strconv.Itoa(idx) + "].price",
				Message: "price must be non-negative",
			})
		}
	}

	if len(details) > 0 {
		return apperrors.NewValidationError("validation failed", details...)
	}

	return nil
}

func (c *ReserveAndAddController) handleUseCaseError(w http.ResponseWriter, traceID string, orderID uint, err error, logger *zap.Logger) {
	if _, ok := apperrors.IsNotFoundError(err); ok {
		c.writeErrorResponse(w, traceID, orderID, http.StatusNotFound, "NOT_FOUND", err.Error(), nil, logger)
		return
	}

	if _, ok := apperrors.IsConflictError(err); ok {
		c.writeErrorResponse(w, traceID, orderID, http.StatusConflict, "CONFLICT", err.Error(), nil, logger)
		return
	}

	if _, ok := apperrors.IsForbiddenError(err); ok {
		c.writeErrorResponse(w, traceID, orderID, http.StatusForbidden, "FORBIDDEN", err.Error(), nil, logger)
		return
	}

	if _, ok := apperrors.IsDeadlockError(err); ok {
		c.writeErrorResponse(w, traceID, orderID, http.StatusConflict, "DEADLOCK", err.Error(), nil, logger)
		return
	}

	logger.Error("unexpected error", zap.Error(err))
	c.writeErrorResponse(w, traceID, orderID, http.StatusInternalServerError, "INTERNAL_ERROR", "an unexpected error occurred", nil, logger)
}

func (c *ReserveAndAddController) writeReserveAndAddResponse(w http.ResponseWriter, traceID string, result *dto.ReservationResult) {
	// Map successes
	successes := make([]dto.ItemSuccessDTO, len(result.Successes))
	addedItems := make([]int, len(result.Successes))
	for i, success := range result.Successes {
		successes[i] = dto.ItemSuccessDTO{
			ProductID: success.ProductID,
			Quantity:  success.Quantity,
		}
		addedItems[i] = success.ProductID
	}

	// Map failures
	failures := make([]dto.ItemFailureDTO, len(result.Failures))
	for i, failure := range result.Failures {
		failures[i] = dto.ItemFailureDTO{
			ProductID: failure.ProductID,
			Quantity:  failure.Quantity,
			Reason:    string(failure.Reason),
		}
	}

	response := dto.ReserveAndAddResponse{
		TraceID:    traceID,
		OrderID:    result.OrderID,
		Status:     string(result.Status),
		TotalPrice: result.TotalPrice,
		AddedItems: addedItems,
		Successes:  successes,
		Failures:   failures,
		Timestamp:  time.Now().UTC(),
	}

	statusCode := http.StatusOK
	if result.Status == dto.ReservationPartial {
		statusCode = http.StatusPartialContent
	} else if result.Status == dto.ReservationAllFailed {
		statusCode = http.StatusUnprocessableEntity
	}

	c.writeJSON(w, statusCode, response)
}

func (c *ReserveAndAddController) writeErrorResponse(w http.ResponseWriter, traceID string, orderID uint, statusCode int, code string, message string, details *dto.ReserveAndAddErrorDetails, logger *zap.Logger) {
	response := dto.ReserveAndAddErrorResponse{
		TraceID:   traceID,
		Status:    statusCode,
		Message:   message,
		Code:      code,
		OrderID:   orderID,
		Details:   details,
		Timestamp: time.Now().UTC(),
	}

	c.writeJSON(w, statusCode, response)
}

type validationErrorResponse struct {
	Error   string                       `json:"error"`
	Message string                       `json:"message"`
	Details []apperrors.ValidationDetail `json:"details"`
}

func (c *ReserveAndAddController) writeValidationError(w http.ResponseWriter, traceID string, message string, details ...apperrors.ValidationDetail) {
	response := validationErrorResponse{
		Error:   "VALIDATION_ERROR",
		Message: message,
		Details: details,
	}

	c.writeJSON(w, http.StatusBadRequest, response)
}

func (c *ReserveAndAddController) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		c.logger.Error("failed to encode response", zap.Error(err))
	}
}
