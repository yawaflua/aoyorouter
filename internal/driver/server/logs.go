package server

import (
	"context"

	"github.com/google/uuid"
	"github.com/yawaflua/aoyorouter/internal/driver/middlewares"
	aoyorouter "github.com/yawaflua/aoyorouter/pkg/pb/api/aoyorouter/docs/api/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// GetProviderLogsByKeyID implements [aoyorouter.AoyoRouterServiceServer].
func (a *AoyoRouterService) GetProviderLogsByKeyID(ctx context.Context, req *aoyorouter.GetProviderLogsByKeyIDRequest) (*aoyorouter.GetProviderLogsByKeyIDResponse, error) {
	requesterKey, ok := middlewares.GetApiKeyFromCtx(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "unauthenticated")
	}
	if requesterKey != nil && !requesterKey.IsAdmin {
		if req.GetApiKeyId() != requesterKey.ID {
			return nil, status.Error(codes.PermissionDenied, "permission denied")
		}
	}

	usage, err := a.UsageEntryRepo.GetUsageEntryByApiKeyID(ctx, uuid.MustParse(req.GetApiKeyId()))
	if err != nil {
		return nil, err
	}
	resp := aoyorouter.GetProviderLogsByKeyIDResponse{}
	for _, v := range usage {
		resp.Logs = append(resp.Logs, &aoyorouter.LogEntry{
			Provider:        v.Provider,
			ApiKeyId:        v.ApiTokenID.String(),
			Latency:         int64(v.Latency),
			InputTokens:     v.InputTokens,
			OutputTokens:    v.OutputTokens,
			TotalTokens:     v.TotalTokens,
			CachedTokens:    v.CachedTokens,
			Model:           v.Model,
			ReasoningEffort: v.Reasoning,
			Failed:          v.Failed,
			Error:           v.Error,
			RequestTime:     timestamppb.New(v.RequestedAt),
			CreatedAt:       timestamppb.New(v.CreatedAt),
		})
		resp.TotalTokens += v.TotalTokens
	}

	return &resp, nil
}

// GetUsageLogs implements [aoyorouter.AoyoRouterServiceServer].
func (a *AoyoRouterService) GetUsageLogs(ctx context.Context, req *aoyorouter.GetUsageLogsRequest) (*aoyorouter.GetUsageLogsResponse, error) {
	requesterKey, ok := middlewares.GetApiKeyFromCtx(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "unauthenticated")
	}


	usage, err := a.UsageEntryRepo.GetAllUsageEntries(ctx, uint64(req.GetLimit()), uint64(req.GetOffset()))
	if err != nil {
		return nil, err
	}
	resp := aoyorouter.GetUsageLogsResponse{}
	for _, v := range usage {
		if requesterKey != nil && requesterKey.ID != v.ApiTokenID.String() {
			continue
		}
		resp.Logs = append(resp.Logs, &aoyorouter.LogEntry{
			Provider:        v.Provider,
			ApiKeyId:        v.ApiTokenID.String(),
			Latency:         int64(v.Latency),
			InputTokens:     v.InputTokens,
			OutputTokens:    v.OutputTokens,
			TotalTokens:     v.TotalTokens,
			CachedTokens:    v.CachedTokens,
			Model:           v.Model,
			ReasoningEffort: v.Reasoning,
			Failed:          v.Failed,
			Error:           v.Error,
			RequestTime:     timestamppb.New(v.RequestedAt),
			CreatedAt:       timestamppb.New(v.CreatedAt),
		})
	}

	return &resp, nil
}
