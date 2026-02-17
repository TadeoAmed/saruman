package dto

import "time"

type ReserveAndAddResponse struct {
	TraceID    string           `json:"traceId"`
	OrderID    uint             `json:"orderId"`
	Status     string           `json:"status"`
	TotalPrice float64          `json:"totalPrice"`
	AddedItems []int            `json:"addedItems"`
	Successes  []ItemSuccessDTO `json:"successes"`
	Failures   []ItemFailureDTO `json:"failures"`
	Timestamp  time.Time        `json:"timestamp"`
}

type ItemSuccessDTO struct {
	ProductID int `json:"productId"`
	Quantity  int `json:"quantity"`
}

type ItemFailureDTO struct {
	ProductID int    `json:"productId"`
	Quantity  int    `json:"quantity"`
	Reason    string `json:"reason"`
}

type ReserveAndAddErrorResponse struct {
	TraceID   string                     `json:"traceId"`
	Status    int                        `json:"status"`
	Message   string                     `json:"message"`
	Code      string                     `json:"code"`
	OrderID   uint                       `json:"orderId"`
	Details   *ReserveAndAddErrorDetails `json:"details,omitempty"`
	Timestamp time.Time                  `json:"timestamp"`
}

type ReserveAndAddErrorDetails struct {
	Failures []ItemFailureDTO `json:"failures"`
}
