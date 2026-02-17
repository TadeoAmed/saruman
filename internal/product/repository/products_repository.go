package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"saruman/internal/domain"
)

type MySQLRepository struct {
	db *sql.DB
}

func NewMySQLRepository(db *sql.DB) *MySQLRepository {
	return &MySQLRepository{db: db}
}

func (r *MySQLRepository) FindByIDsAndCompany(ctx context.Context, ids []int, companyID int) ([]domain.Product, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(ids))
	args := make([]interface{}, 0, len(ids)+1)
	for i, id := range ids {
		placeholders[i] = "?"
		args = append(args, id)
	}
	args = append(args, companyID)

	query := fmt.Sprintf(`
		SELECT id, external_id, name, description, price, stock, reserved_stock,
		       companyId, typeId, category, isActive, isDeleted, hasStock, Stockeable,
		       createdAt, updatedAt
		FROM Product
		WHERE id IN (%s)
		  AND companyId = ?
		  AND isDeleted = 0`,
		strings.Join(placeholders, ", "),
	)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying products: %w", err)
	}
	defer rows.Close()

	var products []domain.Product
	for rows.Next() {
		var p domain.Product
		err := rows.Scan(
			&p.ID, &p.ExternalID, &p.Name, &p.Description, &p.Price,
			&p.Stock, &p.ReservedStock,
			&p.CompanyID, &p.TypeID, &p.Category,
			&p.IsActive, &p.IsDeleted, &p.HasStock, &p.Stockeable,
			&p.CreatedAt, &p.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning product row: %w", err)
		}
		products = append(products, p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating product rows: %w", err)
	}

	return products, nil
}

func (r *MySQLRepository) FindByIDForUpdate(ctx context.Context, tx *sql.Tx, productID int, companyID int) (*domain.Product, error) {
	query := `
		SELECT id, external_id, name, description, price, stock, reserved_stock,
		       companyId, typeId, category, isActive, isDeleted, hasStock, Stockeable,
		       createdAt, updatedAt
		FROM Product
		WHERE id = ?
		  AND companyId = ?
		  AND isDeleted = 0
		FOR UPDATE
	`

	var p domain.Product
	err := tx.QueryRowContext(ctx, query, productID, companyID).Scan(
		&p.ID, &p.ExternalID, &p.Name, &p.Description, &p.Price,
		&p.Stock, &p.ReservedStock,
		&p.CompanyID, &p.TypeID, &p.Category,
		&p.IsActive, &p.IsDeleted, &p.HasStock, &p.Stockeable,
		&p.CreatedAt, &p.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("product with id %d not found: %w", productID, err)
	}
	if err != nil {
		return nil, fmt.Errorf("querying product by id for update: %w", err)
	}

	return &p, nil
}

func (r *MySQLRepository) IncrementReservedStock(ctx context.Context, tx *sql.Tx, productID int, quantity int) error {
	query := `UPDATE Product SET reserved_stock = COALESCE(reserved_stock, 0) + ? WHERE id = ?`

	_, err := tx.ExecContext(ctx, query, quantity, productID)
	if err != nil {
		return fmt.Errorf("incrementing reserved stock: %w", err)
	}

	return nil
}
