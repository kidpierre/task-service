package worker

import (
	"context"
	"log/slog"
	"time"
)

type OverdueMarker interface {
	MarkOverdue(ctx context.Context) (int64, error)
}

type Overdue struct {
	tasks    OverdueMarker
	interval time.Duration
	logger   *slog.Logger
}

func NewOverdue(tasks OverdueMarker, interval time.Duration, logger *slog.Logger) *Overdue {
	return &Overdue{
		tasks:    tasks,
		interval: interval,
		logger:   logger,
	}
}

func (w *Overdue) Run(ctx context.Context) {
	w.logger.Info("overdue worker started", "interval", w.interval.String())
	defer w.logger.Info("overdue worker stopped")

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	w.markOverdue(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.markOverdue(ctx)
		}
	}
}

func (w *Overdue) markOverdue(ctx context.Context) {
	n, err := w.tasks.MarkOverdue(ctx)
	if err != nil {
		if ctx.Err() == nil {
			w.logger.Error("mark overdue tasks", "err", err)
		}
		return
	}
	if n > 0 {
		w.logger.Info("tasks marked as overdue", "count", n)
	}
}
