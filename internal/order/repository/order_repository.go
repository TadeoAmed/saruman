package repository

import (
	"context"
	"database/sql"
	"fmt"

	"saruman/internal/domain"
	"saruman/internal/errors"
)

type MySQLOrderRepository struct {
	db *sql.DB
}

func NewMySQLOrderRepository(db *sql.DB) *MySQLOrderRepository {
	return &MySQLOrderRepository{db: db}
}

func (r *MySQLOrderRepository) FindByID(ctx context.Context, id uint) (*domain.Order, error) {
	query := `
		SELECT id, companyId, firstName, lastName, email, phone, address,
		       status, totalPrice, createdAt, updatedAt
		FROM Orders
		WHERE id = ?
	`

	var order domain.Order
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&order.ID, &order.CompanyID, &order.FirstName, &order.LastName, &order.Email,
		&order.Phone, &order.Address, &order.Status, &order.TotalPrice,
		&order.CreatedAt, &order.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, errors.NewNotFoundError(fmt.Sprintf("order with id %d not found", id))
	}
	if err != nil {
		return nil, fmt.Errorf("querying order by id: %w", err)
	}

	return &order, nil
}

func (r *MySQLOrderRepository) UpdateStatus(ctx context.Context, tx *sql.Tx, id uint, status string) error {
	query := `UPDATE Orders SET status = ? WHERE id = ?`

	result, err := tx.ExecContext(ctx, query, status, id)
	if err != nil {
		return fmt.Errorf("updating order status: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return errors.NewNotFoundError(fmt.Sprintf("order with id %d not found", id))
	}

	return nil
}

func (r *MySQLOrderRepository) UpdateTotalPrice(ctx context.Context, tx *sql.Tx, id uint, totalPrice float64) error {
	query := `UPDATE Orders SET totalPrice = ? WHERE id = ?`

	result, err := tx.ExecContext(ctx, query, totalPrice, id)
	if err != nil {
		return fmt.Errorf("updating order total price: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return errors.NewNotFoundError(fmt.Sprintf("order with id %d not found", id))
	}

	return nil
}
