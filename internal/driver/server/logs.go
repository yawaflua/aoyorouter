package server

import (
	"context"

	"github.com/google/uuid"
	"github.com/yawaflua/aoyorouter/internal/driver/middlewares"
	"github.com/yawaflua/aoyorouter/internal/models"
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
	apiKeyUuid, err := uuid.Parse(req.GetApiKeyId())
	if err != nil {
		return nil, err
	}

	// This RPC has no paging fields; 0 selects the repo's default limit rather
	// than reading the key's entire history.
	usage, err := a.UsageEntryRepo.GetUsageEntriesByApiKeyID(ctx, apiKeyUuid, 0, 0)
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


	// Ownership is filtered in SQL, not after the fact. Applying LIMIT/OFFSET
	// first and then dropping other people's rows in Go meant every page came
	// back short — and an admin holding a real key saw only their own logs,
	// because the check never consulted IsAdmin.
	var (
		usage []*models.UsageEntry
		err   error
	)
	if requesterKey == nil || requesterKey.IsAdmin {
		usage, err = a.UsageEntryRepo.GetAllUsageEntries(ctx, uint64(req.GetLimit()), uint64(req.GetOffset()))
	} else {
		var ownerID uuid.UUID
		ownerID, err = uuid.Parse(requesterKey.ID)
		if err != nil {
			return nil, status.Error(codes.Internal, "malformed api key id")
		}
		usage, err = a.UsageEntryRepo.GetUsageEntriesByApiKeyID(ctx, ownerID, uint64(req.GetLimit()), uint64(req.GetOffset()))
	}
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
