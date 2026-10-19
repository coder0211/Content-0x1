// Package ports declares the use-case-level interfaces shared by several
// modules (SPEC-06 §2.1). Implementations live in internal/platform and
// internal/providers; use cases depend only on these interfaces.
package ports

import "context"

// TxManager runs fn inside a database transaction. The transaction travels in
// the context: repositories pick it up transparently, so every repository
// call made with the ctx passed to fn joins the same transaction. Nested
// calls reuse the outer transaction. fn returning an error rolls back.
type TxManager interface {
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}
