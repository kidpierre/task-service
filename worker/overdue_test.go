package worker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type countingMarker struct {
	calls atomic.Int64
	err   error
}

func (m *countingMarker) MarkOverdue(context.Context) (int64, error) {
	m.calls.Add(1)
	return 1, m.err
}

func TestOverdue_RunsUntilContextCancelled(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "marker succeeds"},
		{name: "marker fails", err: errors.New("db is down")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			marker := &countingMarker{err: tt.err}
			w := NewOverdue(marker, 10*time.Millisecond, slog.New(slog.NewTextHandler(io.Discard, nil)))

			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() {
				w.Run(ctx)
				close(done)
			}()

			time.Sleep(50 * time.Millisecond)
			cancel()

			assert.Eventually(t, func() bool {
				select {
				case <-done:
					return true
				default:
					return false
				}
			}, time.Second, 10*time.Millisecond, "worker did not stop after context cancellation")

			assert.GreaterOrEqual(t, marker.calls.Load(), int64(2))
		})
	}
}
