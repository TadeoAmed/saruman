package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOrderItem_Creation(t *testing.T) {
	item := OrderItem{
		ID:        1,
		OrderID:   100,
		ProductID: 5,
		Quantity:  3,
		Price:     29.99,
	}

	assert.Equal(t, uint(1), item.ID)
	assert.Equal(t, uint(100), item.OrderID)
	assert.Equal(t, 5, item.ProductID)
	assert.Equal(t, 3, item.Quantity)
	assert.Equal(t, 29.99, item.Price)
}

func TestOrderItem_MultipleItems(t *testing.T) {
	items := []OrderItem{
		{
			ID:        1,
			OrderID:   100,
			ProductID: 5,
			Quantity:  2,
			Price:     50.00,
		},
		{
			ID:        2,
			OrderID:   100,
			ProductID: 10,
			Quantity:  1,
			Price:     75.50,
		},
	}

	assert.Len(t, items, 2)
	assert.Equal(t, 50.00, items[0].Price)
	assert.Equal(t, 75.50, items[1].Price)
	assert.Equal(t, uint(100), items[0].OrderID)
	assert.Equal(t, uint(100), items[1].OrderID)
}
