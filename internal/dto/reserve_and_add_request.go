package dto

type ReserveAndAddRequest struct {
	CompanyID int                 `json:"companyId"`
	Items     []ReserveAndAddItem `json:"items"`
}

type ReserveAndAddItem struct {
	ProductID int     `json:"productId"`
	Quantity  int     `json:"quantity"`
	Price     float64 `json:"price"`
}
