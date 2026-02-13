package product

import (
	"encoding/json"
	"net/http"

	apperrors "saruman/internal/errors"

	"go.uber.org/zap"
)

type Controller struct {
	useCase SearchUseCase
	logger  *zap.Logger
}

func NewController(useCase SearchUseCase, logger *zap.Logger) *Controller {
	return &Controller{
		useCase: useCase,
		logger:  logger,
	}
}

func (c *Controller) HandleSearchProducts(w http.ResponseWriter, r *http.Request) {
	var req SearchProductsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		c.writeValidationError(w, "invalid JSON body", apperrors.ValidationDetail{
			Field:   "body",
			Message: "request body must be valid JSON",
		})
		return
	}

	if err := c.validateSearchRequest(req); err != nil {
		ve, _ := apperrors.IsValidationError(err)
		c.writeValidationError(w, ve.Message, ve.Details...)
		return
	}

	resp, err := c.useCase.SearchProducts(r.Context(), req)
	if err != nil {
		c.logger.Error("search products failed", zap.Error(err))
		c.writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error":   "INTERNAL_ERROR",
			"message": "an unexpected error occurred",
		})
		return
	}

	c.writeJSON(w, http.StatusOK, resp)
}

func (c *Controller) validateSearchRequest(req SearchProductsRequest) error {
	if req.CompanyID <= 0 {
		msg := "companyId must be a positive integer"
		if req.CompanyID == 0 {
			msg = "companyId is required"
		}
		return apperrors.NewValidationError(msg, apperrors.ValidationDetail{
			Field:   "companyId",
			Message: msg,
		})
	}

	if len(req.ProductIDs) == 0 {
		msg := "productIds is required"
		return apperrors.NewValidationError(msg, apperrors.ValidationDetail{
			Field:   "productIds",
			Message: "productIds must not be empty",
		})
	}

	if len(req.ProductIDs) > 100 {
		msg := "productIds exceeds maximum of 100"
		return apperrors.NewValidationError(msg, apperrors.ValidationDetail{
			Field:   "productIds",
			Message: msg,
		})
	}

	for _, id := range req.ProductIDs {
		if id <= 0 {
			msg := "each productId must be a positive integer"
			return apperrors.NewValidationError(msg, apperrors.ValidationDetail{
				Field:   "productIds",
				Message: msg,
			})
		}
	}

	return nil
}

type validationErrorResponse struct {
	Error   string                       `json:"error"`
	Message string                       `json:"message"`
	Details []apperrors.ValidationDetail `json:"details"`
}

func (c *Controller) writeValidationError(w http.ResponseWriter, message string, details ...apperrors.ValidationDetail) {
	c.writeJSON(w, http.StatusBadRequest, validationErrorResponse{
		Error:   "VALIDATION_ERROR",
		Message: message,
		Details: details,
	})
}

func (c *Controller) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		c.logger.Error("failed to encode response", zap.Error(err))
	}
}
