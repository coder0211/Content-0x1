package ports

import (
	"context"

	"github.com/be/acs/api/internal/shared/kernel"
)

// AuditRecorder appends to audit_logs. Actor, IP and request ID come from
// ctx (kernel.ActorFrom / kernel.RequestMetaFrom). Call it inside the use
// case's transaction so the audit row commits atomically with the change.
// before/after are marshalled to JSON; pass nil when not applicable.
type AuditRecorder interface {
	Record(ctx context.Context, action, resourceType, resourceID string, before, after any) error
}

// Notification types (notifications.type).
const (
	NotifyJobFailed        = "job_failed"
	NotifyJobSucceeded     = "job_succeeded"
	NotifyReviewRequested  = "review_requested"
	NotifyChangesRequested = "changes_requested"
	NotifyAssetApproved    = "asset_approved"
	NotifyBudgetAlert      = "budget_alert"
)

// Notifier creates in-app notifications.
type Notifier interface {
	Notify(ctx context.Context, userIDs []kernel.ID, notificationType string, payload any) error
}

// Mailer sends plain-text e-mail (budget alerts to Leads).
type Mailer interface {
	Send(ctx context.Context, to []string, subject, body string) error
}
