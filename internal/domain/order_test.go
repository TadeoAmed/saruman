package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestOrder_Creation(t *testing.T) {
	createdAt := time.Now()
	updatedAt := time.Now()
	phone := "1234567890"
	address := "123 Main St"

	order := Order{
		ID:         1,
		CompanyID:  10,
		FirstName:  "John",
		LastName:   "Doe",
		Email:      "john@example.com",
		Phone:      &phone,
		Address:    &address,
		Status:     OrderStatusPending,
		TotalPrice: 99.99,
		CreatedAt:  createdAt,
		UpdatedAt:  updatedAt,
	}

	assert.Equal(t, uint(1), order.ID)
	assert.Equal(t, 10, order.CompanyID)
	assert.Equal(t, "John", order.FirstName)
	assert.Equal(t, "Doe", order.LastName)
	assert.Equal(t, "john@example.com", order.Email)
	assert.Equal(t, &phone, order.Phone)
	assert.Equal(t, &address, order.Address)
	assert.Equal(t, OrderStatusPending, order.Status)
	assert.Equal(t, 99.99, order.TotalPrice)
	assert.Equal(t, createdAt, order.CreatedAt)
	assert.Equal(t, updatedAt, order.UpdatedAt)
}

func TestOrder_NullableFields(t *testing.T) {
	order := Order{
		ID:         1,
		CompanyID:  10,
		FirstName:  "Jane",
		LastName:   "Smith",
		Email:      "jane@example.com",
		Phone:      nil,
		Address:    nil,
		Status:     OrderStatusCreated,
		TotalPrice: 150.00,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	assert.Nil(t, order.Phone)
	assert.Nil(t, order.Address)
}

func TestOrder_StatusConstants(t *testing.T) {
	assert.Equal(t, "PENDING", OrderStatusPending)
	assert.Equal(t, "CREATED", OrderStatusCreated)
	assert.Equal(t, "CANCELED", OrderStatusCanceled)
}
