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
