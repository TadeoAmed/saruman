package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"saruman/internal/config"
	"saruman/internal/platform/logger"
	"saruman/internal/platform/mysql"
	"saruman/internal/product"
	"saruman/internal/server"

	"go.uber.org/zap"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("loading config: %v", err)
	}

	zapLogger, err := logger.New(cfg.Log.Level)
	if err != nil {
		log.Fatalf("creating logger: %v", err)
	}
	defer zapLogger.Sync()

	db, err := mysql.NewConnection(cfg.Database)
	if err != nil {
		zapLogger.Fatal("connecting to database", zap.Error(err))
	}
	defer db.Close()
	zapLogger.Info("database connected")

	productCtrl := product.NewModule(db, zapLogger)

	router := server.NewRouter(productCtrl, zapLogger)

	srv := server.New(cfg.Server.Port, router, zapLogger)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := srv.Start(); err != nil {
			zapLogger.Fatal("server error", zap.Error(err))
		}
	}()

	<-quit
	zapLogger.Info("received shutdown signal")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		zapLogger.Fatal("server shutdown failed", zap.Error(err))
	}

	zapLogger.Info("server stopped gracefully")
}
