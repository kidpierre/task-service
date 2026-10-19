package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"task-service/app/db/pg"
	"task-service/config"
	handlerhttp "task-service/handler/http"
	repopg "task-service/repository/pg"
	"task-service/service"
	"task-service/worker"
)

func Run(cfg *config.Config) error {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := pg.New(ctx, cfg.Postgres)
	if err != nil {
		return fmt.Errorf("init postgres: %w", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.Error("close database", "err", err)
		}
	}()
	logger.Info("database connected and migrated")

	userRepo := repopg.NewUserRepo(db)
	taskRepo := repopg.NewTaskRepo(db)

	authService := service.NewAuthService(userRepo, cfg.JWTSecret, cfg.JWTTTL, logger)
	taskService := service.NewTaskService(taskRepo, logger)

	handler := handlerhttp.New(authService, taskService, logger)
	overdueWorker := worker.NewOverdue(taskService, cfg.OverdueCheckInterval, logger)

	server := &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           handler.Routes(),
		ReadHeaderTimeout: cfg.HTTPReadHeaderTimeout,
		ReadTimeout:       cfg.HTTPReadTimeout,
		WriteTimeout:      cfg.HTTPWriteTimeout,
		IdleTimeout:       cfg.HTTPIdleTimeout,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}

	var wg sync.WaitGroup
	wg.Go(func() {
		overdueWorker.Run(ctx)
	})

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("http server started", "addr", server.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	var runErr error
	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-serverErr:
		runErr = fmt.Errorf("http server: %w", err)
		stop()
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		runErr = errors.Join(runErr, fmt.Errorf("shutdown http server: %w", err))
	}
	wg.Wait()

	logger.Info("service stopped")
	return runErr
}
