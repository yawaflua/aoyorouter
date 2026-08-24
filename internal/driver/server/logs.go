package server

import (
	"context"

	"github.com/google/uuid"
	aoyorouter "github.com/yawaflua/aoyorouter/pkg/pb/api/aoyorouter/docs/api/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// GetProviderLogsByKeyID implements [aoyorouter.AoyoRouterServiceServer].
func (a *AoyoRouterService) GetProviderLogsByKeyID(ctx context.Context, req *aoyorouter.GetProviderLogsByKeyIDRequest) (*aoyorouter.GetProviderLogsByKeyIDResponse, error) {
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
	usage, err := a.UsageEntryRepo.GetAllUsageEntries(ctx, uint64(req.GetLimit()), uint64(req.GetOffset()))
	if err != nil {
		return nil, err
	}
	resp := aoyorouter.GetUsageLogsResponse{}
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
	}

	return &resp, nil
}
