package models

import (
	"time"

	aoyorouter "github.com/yawaflua/aoyorouter/pkg/pb/api/aoyorouter/docs/api/v1"
)

type ApiKey struct {
	ID   string `json:"id"`
	Key  string `json:"key"`
	Name string `json:"name"`

	QuotaSetted         bool        `json:"quota_setted"`
	QuotaTokens         int64       `json:"quota_tokens"`
	QuotaPeriod         QuotaPeriod `json:"quota_period"`
	QuotaResetAt        time.Time   `json:"quota_reset_at"`
	ReservedTokens      int64       `json:"reserved_tokens"`
	RestrictedProviders []string    `json:"restricted_providers"`
	RestrictedModels    []string    `json:"restricted_models"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	IsDeleted bool      `json:"is_deleted"`
	IsActive  bool      `json:"is_active"`
	IsAdmin   bool      `json:"is_admin"`
}

type QuotaPeriod string

const (
	QuotaPeriodForever QuotaPeriod = "forever"
	QuotaPeriodMonth   QuotaPeriod = "month"
	QuotaPeriodWeek    QuotaPeriod = "week"
	QuotaPeriodDay     QuotaPeriod = "day"
	QuotaPeriodHour    QuotaPeriod = "hour"
	QuotaPeriodMinute  QuotaPeriod = "minute"
)

func (q QuotaPeriod) ToDuration() time.Duration {
	switch q {
	case QuotaPeriodForever:
		return time.Hour * 24 * 365 * 10
	case QuotaPeriodMonth:
		return time.Hour * 24 * 30
	case QuotaPeriodWeek:
		return time.Hour * 24 * 7
	case QuotaPeriodDay:
		return time.Hour * 24
	case QuotaPeriodHour:
		return time.Hour
	case QuotaPeriodMinute:
		return time.Minute
	default:
		return 0
	}
}

func ProtoToQuotaPeriod(a *aoyorouter.QuotaResetStrategy) QuotaPeriod {
	switch *a {
	case aoyorouter.QuotaResetStrategy_QUOTA_RESET_STRATEGY_FOREVER:
		return "forever"
	case aoyorouter.QuotaResetStrategy_QUOTA_RESET_STRATEGY_MONTHLY:
		return "month"
	case aoyorouter.QuotaResetStrategy_QUOTA_RESET_STRATEGY_WEEKLY:
		return "week"
	case aoyorouter.QuotaResetStrategy_QUOTA_RESET_STRATEGY_DAILY:
		return "day"
	case aoyorouter.QuotaResetStrategy_QUOTA_RESET_STRATEGY_HOURLY:
		return "hour"
	case aoyorouter.QuotaResetStrategy_QUOTA_RESET_STRATEGY_MINUTES:
		return "minute"
	default:
		return "forever"
	}
}

func (q QuotaPeriod) QuotaToProto(a aoyorouter.QuotaResetStrategy) aoyorouter.QuotaResetStrategy {
	switch q {
	case QuotaPeriodForever:
		return aoyorouter.QuotaResetStrategy_QUOTA_RESET_STRATEGY_FOREVER
	case QuotaPeriodMonth:
		return aoyorouter.QuotaResetStrategy_QUOTA_RESET_STRATEGY_MONTHLY
	case QuotaPeriodWeek:
		return aoyorouter.QuotaResetStrategy_QUOTA_RESET_STRATEGY_WEEKLY
	case QuotaPeriodDay:
		return aoyorouter.QuotaResetStrategy_QUOTA_RESET_STRATEGY_DAILY
	case QuotaPeriodHour:
		return aoyorouter.QuotaResetStrategy_QUOTA_RESET_STRATEGY_HOURLY
	case QuotaPeriodMinute:
		return aoyorouter.QuotaResetStrategy_QUOTA_RESET_STRATEGY_MINUTES
	default:
		return aoyorouter.QuotaResetStrategy_QUOTA_RESET_STRATEGY_FOREVER
	}
}
