// Package usecase implements the audit module: append-only audit log
// (FR-OPS-06) and in-app notifications. Other modules depend on the
// ports.AuditRecorder / ports.Notifier interfaces this package implements.
package usecase

import "github.com/be/acs/api/internal/ports"

// API is what the audit module exposes to other modules.
type API interface {
	ports.AuditRecorder
	ports.Notifier
}
