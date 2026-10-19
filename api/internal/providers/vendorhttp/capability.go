package vendorhttp

import (
	"slices"

	"github.com/be/acs/api/internal/shared/kernel"
)

// Unsupported builds the 422 CAPABILITY_NOT_SUPPORTED error adapters return
// when a request parameter is outside the model's capabilities (ACS-050 AC):
// details carry the field, the rejected value and the supported values.
// It is a DomainError, so the job engine does not retry it.
func Unsupported(field string, value any, supported any) *kernel.DomainError {
	return kernel.ErrCapability.WithDetails(kernel.Detail{
		"field": field, "issue": "unsupported", "value": value, "supported": supported,
	})
}

// CheckOneOf returns Unsupported when v is set and not in allowed (an empty
// allowed list means "anything").
func CheckOneOf(field, v string, allowed []string) error {
	if v == "" || len(allowed) == 0 || slices.Contains(allowed, v) {
		return nil
	}
	return Unsupported(field, v, allowed)
}
