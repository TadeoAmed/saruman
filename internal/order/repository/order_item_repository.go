package repository

import (
	"context"
	"database/sql"
	"fmt"

	"saruman/internal/domain"
)

type MySQLOrderItemRepository struct {
	db *sql.DB
}

func NewMySQLOrderItemRepository(db *sql.DB) *MySQLOrderItemRepository {
	return &MySQLOrderItemRepository{db: db}
}

func (r *MySQLOrderItemRepository) Insert(ctx context.Context, tx *sql.Tx, item domain.OrderItem) (uint, error) {
	query := `INSERT INTO OrderItems (orderId, productId, quantity, price) VALUES (?, ?, ?, ?)`

	result, err := tx.ExecContext(ctx, query, item.OrderID, item.ProductID, item.Quantity, item.Price)
	if err != nil {
		return 0, fmt.Errorf("inserting order item: %w", err)
	}

	lastInsertID, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("getting last insert id: %w", err)
	}

	return uint(lastInsertID), nil
}
