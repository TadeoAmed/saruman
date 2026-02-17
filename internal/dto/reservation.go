package dto

type ReservationStatus string

const (
	ReservationAllSuccess ReservationStatus = "ALL_SUCCESS"
	ReservationPartial    ReservationStatus = "PARTIAL"
	ReservationAllFailed  ReservationStatus = "ALL_FAILED"
)

type FailureReason string

const (
	ReasonNotFound              FailureReason = "NOT_FOUND"
	ReasonOutOfStock            FailureReason = "OUT_OF_STOCK"
	ReasonInsufficientAvailable FailureReason = "INSUFFICIENT_AVAILABLE"
	ReasonProductInactive       FailureReason = "PRODUCT_INACTIVE"
	ReasonProductNotStockeable  FailureReason = "PRODUCT_NOT_STOCKEABLE"
)

type ItemSuccess struct {
	ProductID int
	Quantity  int
}

type ItemFailure struct {
	ProductID int
	Quantity  int
	Reason    FailureReason
}

type ReservationResult struct {
	Status     ReservationStatus
	OrderID    uint
	TotalPrice float64
	Successes  []ItemSuccess
	Failures   []ItemFailure
}

type ReservationItem struct {
	ProductID int
	Quantity  int
	Price     float64
}
