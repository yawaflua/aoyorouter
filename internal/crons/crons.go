package crons

import (
	"fmt"
	"log/slog"

	"github.com/robfig/cron/v3"
	"github.com/yawaflua/aoyorouter/internal/closer"
)

type Crons struct {
	Name     string
	Interval string
	Handler  func() error
	Closer   *closer.C
	Logger   *slog.Logger
}

func (r *Crons) Run() error {
	c := cron.New()

	_, err := c.AddFunc(r.Interval, func() {
		r.Logger.Info("Executing cron job", "name", r.Name)
		if err := r.Handler(); err != nil {
			r.Logger.Error("cron job failed", "error", err)
		}
	})
	if err != nil {
		r.Logger.Error("Error adding cron job:", slog.Any("error", err))
		return err
	}
	err = r.Handler()
	if err != nil {
		return err
	}
	c.Start()
	r.Closer.Add(func() error {
		c.Stop()
		return nil
	})
	return nil
}
