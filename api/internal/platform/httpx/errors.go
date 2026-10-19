package httpx

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"

	"github.com/be/acs/api/internal/shared/kernel"
)

var (
	statusMu sync.RWMutex
	// statusByCode maps error codes to HTTP statuses (SPEC-04 §1.2).
	statusByCode = map[string]int{
		kernel.CodeValidation:             http.StatusBadRequest,
		kernel.CodeIdempotencyKeyRequired: http.StatusBadRequest,
		kernel.CodeUnauthenticated:        http.StatusUnauthorized,
		kernel.CodeSessionExpired:         http.StatusUnauthorized,
		kernel.CodeInvalidCredentials:     http.StatusUnauthorized,
		kernel.CodeForbidden:              http.StatusForbidden,
		kernel.CodeCSRFInvalid:            http.StatusForbidden,
		kernel.CodeSelfApproval:           http.StatusForbidden,
		kernel.CodeAccountDisabled:        http.StatusForbidden,
		kernel.CodeNotFound:               http.StatusNotFound,
		kernel.CodeConflict:               http.StatusConflict,
		kernel.CodeInvalidStateTransition: http.StatusConflict,
		kernel.CodeIdempotencyConflict:    http.StatusConflict,
		kernel.CodeVersionConflict:        http.StatusConflict,
		kernel.CodeFileTooLarge:           http.StatusRequestEntityTooLarge,
		kernel.CodeUnsupportedMediaType:   http.StatusUnsupportedMediaType,
		kernel.CodeCapabilityNotSupported: http.StatusUnprocessableEntity,
		kernel.CodeAccountLocked:          http.StatusLocked,
		kernel.CodeRateLimited:            http.StatusTooManyRequests,
		kernel.CodeBudgetExceeded:         http.StatusTooManyRequests,
		kernel.CodeJobBudgetExceeded:      http.StatusTooManyRequests,
		kernel.CodeConcurrencyLimit:       http.StatusTooManyRequests,
		kernel.CodeProviderError:          http.StatusBadGateway,
		kernel.CodeServiceUnavailable:     http.StatusServiceUnavailable,
		kernel.CodePricingMissing:         http.StatusServiceUnavailable,
		kernel.CodeInternal:               http.StatusInternalServerError,
	}
)

// RegisterErrorCode maps a module-specific error code to an HTTP status.
// Call it from the module's adapter/http package init. Re-registering a code
// with a different status panics (two modules disagreeing is a bug).
func RegisterErrorCode(code string, status int) {
	statusMu.Lock()
	defer statusMu.Unlock()
	if s, ok := statusByCode[code]; ok && s != status {
		panic("httpx: error code " + code + " already mapped to a different status")
	}
	statusByCode[code] = status
}

// StatusFor returns the HTTP status for a domain error code; unknown codes
// are client errors (400).
func StatusFor(code string) int {
	statusMu.RLock()
	defer statusMu.RUnlock()
	if s, ok := statusByCode[code]; ok {
		return s
	}
	return http.StatusBadRequest
}

// Error writes err as an error envelope. Domain errors keep their code,
// message and details; anything else becomes a logged 500 INTERNAL whose
// cause never reaches the client.
func Error(w http.ResponseWriter, r *http.Request, err error) {
	de, ok := kernel.AsDomainError(err)
	if !ok {
		if errors.Is(err, context.Canceled) {
			// Client went away; nothing useful to send.
			return
		}
		loggerFrom(r.Context()).ErrorContext(r.Context(), "unhandled error", slog.String("error", err.Error()))
		de = kernel.ErrInternal
	}
	status := StatusFor(de.Code)
	if status >= 500 && ok {
		loggerFrom(r.Context()).ErrorContext(r.Context(), "server error", slog.String("code", de.Code), slog.String("error", err.Error()))
	}
	writeError(w, r, status, de)
}

func writeError(w http.ResponseWriter, r *http.Request, status int, de *kernel.DomainError) {
	details := de.Details
	if details == nil {
		details = []kernel.Detail{}
	}
	writeJSON(w, status, errorEnvelope{
		Error: errorBody{Code: de.Code, Message: de.Message, Details: details},
		Meta:  Meta{RequestID: RequestID(r.Context())},
	})
}
