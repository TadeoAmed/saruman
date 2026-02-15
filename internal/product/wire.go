package product

import (
	"database/sql"

	"saruman/internal/product/controller"
	"saruman/internal/product/repository"
	"saruman/internal/product/service"
	"saruman/internal/product/usecase"

	"go.uber.org/zap"
)

func NewModule(db *sql.DB, logger *zap.Logger) *controller.Controller {
	repo := repository.NewMySQLRepository(db)
	svc := service.NewService(repo)
	uc := usecase.NewSearchUseCase(svc)
	return controller.NewController(uc, logger)
}
