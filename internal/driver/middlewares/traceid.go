package middlewares

import (
	"log/slog"
	"net/http"

	"github.com/google/uuid"
	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/yawaflua/aoyorouter/pkg/logger"
)

const (
	requestIDMetadataName = "X-Trace-Id"
)

func RequestIDInterceptor(next runtime.HandlerFunc) runtime.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request, pathParams map[string]string) {
		var requestID string

		requestID = r.Header.Get(requestIDMetadataName)
		if requestID == "" {
			requestID = uuid.New().String()
		}

		log := logger.GetLoggerFromCtx(r.Context()).With(
			slog.String("trace_id", requestID),
		)

		ctx := logger.SetLoggerToCtx(r.Context(), log)

		next(w, r.WithContext(ctx), pathParams)
	}
}
