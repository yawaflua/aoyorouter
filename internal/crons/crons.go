package crons

import (
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
	log := r.Logger
	if log == nil {
		log = slog.Default()
	}

	c := cron.New()

	run := func() {
		log.Info("Executing cron job", "name", r.Name)
		if err := r.Handler(); err != nil {
			log.Error("cron job failed", "name", r.Name, "error", err)
		}
	}

	if _, err := c.AddFunc(r.Interval, run); err != nil {
		log.Error("Error adding cron job:", slog.String("name", r.Name), slog.Any("error", err))
		return err
	}

	c.Start()
	if r.Closer != nil {
		r.Closer.Add(func() error {
			c.Stop()
			return nil
		})
	}

	// The initial pass runs detached. It used to run synchronously and its
	// error was returned all the way up through initCrons -> initDeps, so a
	// transient database hiccup — or one slow provider, since quota_loader
	// makes a network call per provider — stopped the process from starting
	// at all, and left the schedule unregistered on top of that.
	go run()

	return nil
}
