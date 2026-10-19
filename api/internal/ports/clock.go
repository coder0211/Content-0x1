package ports

import "time"

// Clock abstracts time so use cases are deterministic under test.
type Clock interface {
	Now() time.Time
}

// SystemClock is the production Clock (UTC wall time).
type SystemClock struct{}

// Now returns the current UTC time.
func (SystemClock) Now() time.Time { return time.Now().UTC() }
