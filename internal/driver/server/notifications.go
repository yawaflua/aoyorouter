package server

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/yawaflua/aoyorouter/internal/driver/middlewares"
	"github.com/yawaflua/aoyorouter/internal/models"
	aoyorouter "github.com/yawaflua/aoyorouter/pkg/pb/api/aoyorouter/docs/api/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const notificationEventsLimit = 50

func (a *AoyoRouterService) Subscribe(ctx context.Context, req *aoyorouter.SubscribeRequest) (*aoyorouter.SubscribeResponse, error) {
	requesterKey, ok := middlewares.GetApiKeyFromCtx(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "unauthenticated")
	}

	subject := strings.TrimSpace(req.GetSubject())
	if subject == "" {
		return nil, status.Error(codes.InvalidArgument, "subject is required")
	}

	subscription := req.GetSubscription()
	if subscription == nil || strings.TrimSpace(subscription.GetEndpoint()) == "" {
		return nil, status.Error(codes.InvalidArgument, "subscription endpoint is required")
	}
	if !models.IsInAppEndpoint(subscription.GetEndpoint()) {
		if subscription.GetKeys() == nil || subscription.GetKeys().GetP256Dh() == "" || subscription.GetKeys().GetAuth() == "" {
			return nil, status.Error(codes.InvalidArgument, "subscription keys are required")
		}
	}

	if providerID, isQuota := models.ProviderIDFromQuotaTopic(subject); isQuota {
		if requesterKey != nil && !requesterKey.IsAdmin {
			if slices.Contains(requesterKey.RestrictedProviders, providerID) {
				return nil, status.Error(codes.PermissionDenied, "permission denied")
			}
		}
		if _, err := a.ProviderRepo.GetProvider(ctx, providerID); err != nil {
			return nil, status.Error(codes.NotFound, "provider not found")
		}
	}

	sub := &models.PushSubscription{
		Endpoint: subscription.GetEndpoint(),
		Keys: models.PushKeys{
			P256dh: subscription.GetKeys().GetP256Dh(),
			Auth:   subscription.GetKeys().GetAuth(),
		},
		ExpirationTime: subscription.ExpirationTime,
		UserAgent:      req.GetUserAgent(),
		Labels:         req.GetLabels(),
	}

	id, created, err := a.pushRepo.Subscribe(ctx, subject, sub)
	if err != nil {
		a.logger.Error("failed to store push subscription", "subject", subject, "error", err)
		return nil, status.Error(codes.Internal, "failed to store subscription")
	}

	return &aoyorouter.SubscribeResponse{SubscriptionId: id, Created: created}, nil
}

func (a *AoyoRouterService) Unsubscribe(ctx context.Context, req *aoyorouter.UnsubscribeRequest) (*aoyorouter.UnsubscribeResponse, error) {
	if _, ok := middlewares.GetApiKeyFromCtx(ctx); !ok {
		return nil, status.Error(codes.Unauthenticated, "unauthenticated")
	}

	subject := strings.TrimSpace(req.GetSubject())
	if subject == "" {
		return nil, status.Error(codes.InvalidArgument, "subject is required")
	}
	endpoint := strings.TrimSpace(req.GetEndpoint())
	if endpoint == "" {
		return nil, status.Error(codes.InvalidArgument, "endpoint is required")
	}

	removed, err := a.pushRepo.Unsubscribe(ctx, subject, endpoint)
	if err != nil {
		a.logger.Error("failed to remove push subscription", "subject", subject, "error", err)
		return nil, status.Error(codes.Internal, "failed to remove subscription")
	}

	return &aoyorouter.UnsubscribeResponse{Removed: removed}, nil
}

func (a *AoyoRouterService) ListSubscriptions(ctx context.Context, req *aoyorouter.ListSubscriptionsRequest) (*aoyorouter.ListSubscriptionsResponse, error) {
	if _, ok := middlewares.GetApiKeyFromCtx(ctx); !ok {
		return nil, status.Error(codes.Unauthenticated, "unauthenticated")
	}

	endpoint := strings.TrimSpace(req.GetEndpoint())
	if endpoint == "" {
		return nil, status.Error(codes.InvalidArgument, "endpoint is required")
	}

	subjects, err := a.pushRepo.SubjectsByEndpoint(ctx, endpoint)
	if err != nil {
		a.logger.Error("failed to list push subscriptions", "error", err)
		return nil, status.Error(codes.Internal, "failed to list subscriptions")
	}

	return &aoyorouter.ListSubscriptionsResponse{Subjects: subjects}, nil
}

func (a *AoyoRouterService) ListNotificationEvents(ctx context.Context, req *aoyorouter.ListNotificationEventsRequest) (*aoyorouter.ListNotificationEventsResponse, error) {
	if _, ok := middlewares.GetApiKeyFromCtx(ctx); !ok {
		return nil, status.Error(codes.Unauthenticated, "unauthenticated")
	}

	endpoint := strings.TrimSpace(req.GetEndpoint())
	if endpoint == "" {
		return nil, status.Error(codes.InvalidArgument, "endpoint is required")
	}

	events, err := a.pushRepo.EventsForEndpoint(ctx, endpoint, req.GetAfterId(), notificationEventsLimit)
	if err != nil {
		a.logger.Error("failed to list notification events", "error", err)
		return nil, status.Error(codes.Internal, "failed to list notification events")
	}

	lastID := req.GetAfterId()
	items := make([]*aoyorouter.NotificationEvent, 0, len(events))
	for _, ev := range events {
		items = append(items, &aoyorouter.NotificationEvent{
			Id:         ev.ID,
			Subject:    ev.Subject,
			Title:      ev.Title,
			Body:       ev.Body,
			Tag:        ev.Tag,
			ProviderId: ev.ProviderID,
			Url:        ev.URL,
			CreatedAt:  ev.CreatedAt.Format(time.RFC3339),
		})
		if ev.ID > lastID {
			lastID = ev.ID
		}
	}

	return &aoyorouter.ListNotificationEventsResponse{Events: items, LastId: lastID}, nil
}

func (a *AoyoRouterService) GetPushConfig(ctx context.Context, _ *aoyorouter.GetPushConfigRequest) (*aoyorouter.GetPushConfigResponse, error) {
	if _, ok := middlewares.GetApiKeyFromCtx(ctx); !ok {
		return nil, status.Error(codes.Unauthenticated, "unauthenticated")
	}

	if a.notifier == nil {
		return &aoyorouter.GetPushConfigResponse{Enabled: false}, nil
	}

	publicKey, err := a.notifier.PublicKey(ctx)
	if err != nil {
		a.logger.Error("failed to resolve vapid public key", "error", err)
		return &aoyorouter.GetPushConfigResponse{Enabled: false}, nil
	}

	return &aoyorouter.GetPushConfigResponse{VapidPublicKey: publicKey, Enabled: publicKey != ""}, nil
}
