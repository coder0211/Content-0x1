package domain

import (
	"time"

	"github.com/be/acs/api/internal/shared/kernel"
)

// SessionPolicy holds idle and absolute timeouts (12h / 7d).
type SessionPolicy struct {
	Idle     time.Duration
	Absolute time.Duration
}

// TouchInterval limits last_seen_at writes to one per interval per session.
const TouchInterval = time.Minute

// Session is a login session; only the SHA-256 of its token is stored.
type Session struct {
	ID         kernel.ID
	UserID     kernel.ID
	TokenHash  []byte
	CSRFToken  string
	IP         string
	UserAgent  string
	LastSeenAt time.Time
	ExpiresAt  time.Time
	RevokedAt  *time.Time
	CreatedAt  time.Time
}

// NewSession starts a session at now.
func NewSession(userID kernel.ID, tokenHash []byte, csrf, ip, ua string, now time.Time, p SessionPolicy) *Session {
	return &Session{
		ID: kernel.NewID(), UserID: userID, TokenHash: tokenHash, CSRFToken: csrf, IP: ip, UserAgent: ua,
		LastSeenAt: now, ExpiresAt: now.Add(p.Absolute), CreatedAt: now,
	}
}

// Validate returns SESSION_EXPIRED when revoked, past the absolute expiry,
// or idle for longer than the idle timeout.
func (s *Session) Validate(now time.Time, p SessionPolicy) error {
	if s.RevokedAt != nil || !now.Before(s.ExpiresAt) || now.Sub(s.LastSeenAt) >= p.Idle {
		return kernel.ErrSessionExpired
	}
	return nil
}

// NeedsTouch reports whether last_seen_at should be refreshed.
func (s *Session) NeedsTouch(now time.Time) bool { return now.Sub(s.LastSeenAt) >= TouchInterval }
