package product

import (
	"database/sql"

	"go.uber.org/zap"
)

func NewModule(db *sql.DB, logger *zap.Logger) *Controller {
	repo := NewMySQLRepository(db)
	svc := NewService(repo)
	uc := NewSearchUseCase(svc)
	return NewController(uc, logger)
}
