package domain

import "time"

type CompanyConfig struct {
	ID                int
	CompanyID         int
	FieldsOrderConfig string
	HasStock          bool
	CreatedAt         time.Time
	UpdatedAt         time.Time
}
