package kernel

import (
	"errors"
	"strings"
)

// Error codes shared across modules (SPEC-04 §1.2). The HTTP layer maps each
// code to a status; modules may define more codes and register them with
// httpx.RegisterErrorCode.
const (
	CodeValidation             = "VALIDATION_FAILED"
	CodeUnauthenticated        = "UNAUTHENTICATED"
	CodeSessionExpired         = "SESSION_EXPIRED"
	CodeInvalidCredentials     = "INVALID_CREDENTIALS" //nolint:gosec // G101: an error code, not a credential
	CodeForbidden              = "FORBIDDEN"
	CodeCSRFInvalid            = "CSRF_INVALID"
	CodeSelfApproval           = "SELF_APPROVAL_NOT_ALLOWED"
	CodeAccountDisabled        = "ACCOUNT_DISABLED"
	CodeNotFound               = "NOT_FOUND"
	CodeConflict               = "CONFLICT"
	CodeInvalidStateTransition = "INVALID_STATE_TRANSITION"
	CodeIdempotencyConflict    = "IDEMPOTENCY_CONFLICT"
	CodeIdempotencyKeyRequired = "IDEMPOTENCY_KEY_REQUIRED"
	CodeVersionConflict        = "VERSION_CONFLICT"
	CodeFileTooLarge           = "FILE_TOO_LARGE"
	CodeUnsupportedMediaType   = "UNSUPPORTED_MEDIA_TYPE"
	CodeCapabilityNotSupported = "CAPABILITY_NOT_SUPPORTED"
	CodeAccountLocked          = "ACCOUNT_LOCKED"
	CodeRateLimited            = "RATE_LIMITED"
	CodeBudgetExceeded         = "BUDGET_EXCEEDED"
	CodeJobBudgetExceeded      = "JOB_BUDGET_EXCEEDED"
	CodeConcurrencyLimit       = "CONCURRENCY_LIMIT"
	CodeProviderError          = "PROVIDER_ERROR"
	CodeServiceUnavailable     = "SERVICE_UNAVAILABLE"
	CodePricingMissing         = "PRICING_MISSING"
	CodeInternal               = "INTERNAL"
)

// Detail is one structured entry of DomainError.Details, e.g.
// {"field": "brief.target_audience", "issue": "required"} or
// {"field": "aspect_ratio", "supported": ["1:1", "16:9"]}.
type Detail map[string]any

// FieldIssue builds the common {"field", "issue"} detail.
func FieldIssue(field, issue string) Detail {
	return Detail{"field": field, "issue": issue}
}

// DomainError is the single error type that crosses layer boundaries. Code is
// machine-readable (FE switches on it); Message is Vietnamese, user-facing.
type DomainError struct {
	Code    string
	Message string
	Details []Detail
	cause   error
}

// NewError creates a DomainError.
func NewError(code, message string, details ...Detail) *DomainError {
	return &DomainError{Code: code, Message: message, Details: details}
}

func (e *DomainError) Error() string {
	var b strings.Builder
	b.WriteString(e.Code)
	if e.Message != "" {
		b.WriteString(": ")
		b.WriteString(e.Message)
	}
	if e.cause != nil {
		b.WriteString(": ")
		b.WriteString(e.cause.Error())
	}
	return b.String()
}

// Unwrap exposes the wrapped cause, if any.
func (e *DomainError) Unwrap() error { return e.cause }

// Is matches any DomainError with the same code, so callers can write
// errors.Is(err, kernel.ErrNotFound) regardless of message or details.
func (e *DomainError) Is(target error) bool {
	var t *DomainError
	if errors.As(target, &t) {
		return t.Code == e.Code
	}
	return false
}

// WithDetails returns a copy with details appended.
func (e *DomainError) WithDetails(d ...Detail) *DomainError {
	c := *e
	c.Details = append(append([]Detail(nil), e.Details...), d...)
	return &c
}

// WithMessage returns a copy with a different message.
func (e *DomainError) WithMessage(msg string) *DomainError {
	c := *e
	c.Message = msg
	return &c
}

// Wrap returns a copy that carries cause for logging. The cause is never sent
// to clients.
func (e *DomainError) Wrap(cause error) *DomainError {
	c := *e
	c.cause = cause
	return &c
}

// Sentinel errors. Compare with errors.Is; derive variants with WithMessage /
// WithDetails rather than mutating them.
var (
	ErrValidation          = NewError(CodeValidation, "Dữ liệu không hợp lệ")
	ErrUnauthenticated     = NewError(CodeUnauthenticated, "Vui lòng đăng nhập")
	ErrSessionExpired      = NewError(CodeSessionExpired, "Phiên đăng nhập đã hết hạn, vui lòng đăng nhập lại")
	ErrInvalidCredentials  = NewError(CodeInvalidCredentials, "Email hoặc mật khẩu không đúng")
	ErrForbidden           = NewError(CodeForbidden, "Bạn không có quyền thực hiện thao tác này")
	ErrCSRFInvalid         = NewError(CodeCSRFInvalid, "Phiên làm việc không hợp lệ, vui lòng tải lại trang")
	ErrNotFound            = NewError(CodeNotFound, "Không tìm thấy dữ liệu")
	ErrConflict            = NewError(CodeConflict, "Dữ liệu đã tồn tại hoặc bị xung đột")
	ErrInvalidTransition   = NewError(CodeInvalidStateTransition, "Không thể chuyển sang trạng thái này")
	ErrIdempotencyConflict = NewError(CodeIdempotencyConflict, "Idempotency-Key đã được dùng cho một yêu cầu khác")
	ErrVersionConflict     = NewError(CodeVersionConflict, "Dữ liệu đã được người khác cập nhật, vui lòng tải lại")
	ErrFileTooLarge        = NewError(CodeFileTooLarge, "Tệp vượt quá dung lượng cho phép")
	ErrUnsupportedMedia    = NewError(CodeUnsupportedMediaType, "Định dạng tệp không được hỗ trợ")
	ErrCapability          = NewError(CodeCapabilityNotSupported, "Model không hỗ trợ tham số đã chọn")
	ErrAccountLocked       = NewError(CodeAccountLocked, "Tài khoản tạm thời bị khóa do đăng nhập sai nhiều lần")
	ErrRateLimited         = NewError(CodeRateLimited, "Bạn thao tác quá nhanh, vui lòng thử lại sau")
	ErrBudgetExceeded      = NewError(CodeBudgetExceeded, "Đã vượt hạn mức ngân sách")
	ErrJobBudgetExceeded   = NewError(CodeJobBudgetExceeded, "Chi phí dự toán vượt hạn mức cho một tác vụ")
	ErrConcurrencyLimit    = NewError(CodeConcurrencyLimit, "Đã đạt giới hạn số tác vụ chạy đồng thời")
	ErrProvider            = NewError(CodeProviderError, "Nhà cung cấp AI gặp lỗi, vui lòng thử lại")
	ErrUnavailable         = NewError(CodeServiceUnavailable, "Hệ thống tạm thời không khả dụng")
	ErrPricingMissing      = NewError(CodePricingMissing, "Chưa có bảng giá hiệu lực cho model này")
	ErrInternal            = NewError(CodeInternal, "Đã có lỗi xảy ra, vui lòng thử lại")
)

// Validation builds a VALIDATION_FAILED error with the given details.
func Validation(details ...Detail) *DomainError {
	return ErrValidation.WithDetails(details...)
}

// InvalidTransition builds an INVALID_STATE_TRANSITION error describing from → to.
func InvalidTransition(from, to string) *DomainError {
	return ErrInvalidTransition.WithDetails(Detail{"from": from, "to": to})
}

// AsDomainError extracts a *DomainError from err's chain.
func AsDomainError(err error) (*DomainError, bool) {
	var de *DomainError
	if errors.As(err, &de) {
		return de, true
	}
	return nil, false
}

// ErrorCode returns err's domain code, or "" for non-domain errors.
func ErrorCode(err error) string {
	if de, ok := AsDomainError(err); ok {
		return de.Code
	}
	return ""
}

// ValidationErrors accumulates field issues so a use case can report every
// problem at once (FR-CMP-01: "400 với chi tiết từng field").
type ValidationErrors struct{ details []Detail }

// Add records an issue for field.
func (v *ValidationErrors) Add(field, issue string) {
	v.details = append(v.details, FieldIssue(field, issue))
}

// AddDetail records an arbitrary detail.
func (v *ValidationErrors) AddDetail(d Detail) { v.details = append(v.details, d) }

// Err returns nil when nothing was recorded, else a VALIDATION_FAILED error.
func (v *ValidationErrors) Err() error {
	if len(v.details) == 0 {
		return nil
	}
	return Validation(v.details...)
}
