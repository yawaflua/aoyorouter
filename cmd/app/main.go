package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/yawaflua/aoyorouter/internal/app"
)


func main() {
	ctx := context.Background()
	
	a, err := app.New(ctx)
	if err != nil {
		panic(err)
	}

	go func() {
		if err := a.Start(ctx); err != nil {
			if errors.Is(err, http.ErrServerClosed) {
				return
			}
			panic(err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)

	<-quit

	if err := a.GracefullyStop(ctx); err != nil {
		panic(err)
	}
}
