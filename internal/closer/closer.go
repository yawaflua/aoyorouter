package closer

import (
	"errors"
	"log/slog"
	"sync"
	"time"
)

// ErrCloseTimeout is returned by WaitTimeout when shutdown did not finish in
// the allotted time.
var ErrCloseTimeout = errors.New("closer: timed out waiting for shutdown")

type C struct {
	mu    sync.Mutex
	once  sync.Once
	done  chan struct{}
	funcs []func() error
	log   *slog.Logger
}

func New(log *slog.Logger) *C {
	c := &C{
		done: make(chan struct{}),
		log:  log,
	}
	return c
}

func (c *C) Add(f ...func() error) {
	c.mu.Lock()
	c.funcs = append(c.funcs, f...)
	c.mu.Unlock()
}

func (c *C) Wait() {
	<-c.done
}

// WaitTimeout blocks until every close function has returned, or until d
// elapses. Without a bound a single close func that never returns — a hung
// database handle, an in-flight upstream request — wedges shutdown forever.
func (c *C) WaitTimeout(d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-c.done:
		return nil
	case <-timer.C:
		c.log.Error("shutdown timed out, exiting anyway", slog.Duration("timeout", d))
		return ErrCloseTimeout
	}
}

func (c *C) CloseAll() {
	c.log.Info("gracefully stops")
	c.once.Do(func() {
		defer close(c.done)

		c.mu.Lock()
		funcs := c.funcs
		c.funcs = nil
		c.mu.Unlock()

		errs := make(chan error, len(funcs))
		for _, f := range funcs {
			go func(f func() error) {
				errs <- f()
			}(f)
		}

		for i := 0; i < cap(errs); i++ {
			if err := <-errs; err != nil {
				c.log.Error("failed to close", slog.String("error", err.Error()))
			}
		}
	})
}