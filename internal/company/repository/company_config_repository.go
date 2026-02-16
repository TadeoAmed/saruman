package repository

import (
	"context"
	"database/sql"
	"fmt"

	"saruman/internal/domain"
	"saruman/internal/errors"
)

type MySQLCompanyConfigRepository struct {
	db *sql.DB
}

func NewMySQLCompanyConfigRepository(db *sql.DB) *MySQLCompanyConfigRepository {
	return &MySQLCompanyConfigRepository{db: db}
}

func (r *MySQLCompanyConfigRepository) FindByCompanyID(ctx context.Context, companyID int) (*domain.CompanyConfig, error) {
	query := `
		SELECT id, companyId, fieldsOrderConfig, hasStock, createdAt, updatedAt
		FROM CompanyConfig
		WHERE companyId = ?
	`

	var config domain.CompanyConfig
	err := r.db.QueryRowContext(ctx, query, companyID).Scan(
		&config.ID, &config.CompanyID, &config.FieldsOrderConfig, &config.HasStock,
		&config.CreatedAt, &config.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, errors.NewNotFoundError(fmt.Sprintf("company config for company id %d not found", companyID))
	}
	if err != nil {
		return nil, fmt.Errorf("querying company config by company id: %w", err)
	}

	return &config, nil
}
