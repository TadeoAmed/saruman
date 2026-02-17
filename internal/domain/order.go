package domain

import "time"

type Order struct {
	ID         uint
	CompanyID  int
	FirstName  string
	LastName   string
	Email      string
	Phone      *string
	Address    *string
	Status     string
	TotalPrice float64
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

const (
	OrderStatusPending  = "PENDING"
	OrderStatusCreated  = "CREATED"
	OrderStatusCanceled = "CANCELED"
)
