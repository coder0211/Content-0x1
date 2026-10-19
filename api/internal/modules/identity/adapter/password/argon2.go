// Package password implements usecase.PasswordHasher with argon2id
// (PHC string format, OWASP-recommended parameters).
package password

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Params are argon2id cost parameters.
type Params struct {
	Memory  uint32 // KiB
	Time    uint32
	Threads uint8
	KeyLen  uint32
	SaltLen uint32
}

// DefaultParams: 64 MiB, 3 passes, 2 lanes.
var DefaultParams = Params{Memory: 64 * 1024, Time: 3, Threads: 2, KeyLen: 32, SaltLen: 16}

// Argon2id hashes passwords.
type Argon2id struct{ P Params }

// New returns a hasher with DefaultParams.
func New() *Argon2id { return &Argon2id{P: DefaultParams} }

// Hash returns $argon2id$v=19$m=..,t=..,p=..$salt$key.
func (a *Argon2id) Hash(pw string) (string, error) {
	salt := make([]byte, a.P.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(pw), salt, a.P.Time, a.P.Memory, a.P.Threads, a.P.KeyLen)
	b64 := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, a.P.Memory, a.P.Time, a.P.Threads,
		b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

// ErrMalformedHash is returned for unparsable encodings.
var ErrMalformedHash = errors.New("malformed argon2id hash")

// Verify checks pw against an encoded hash in constant time; the encoded
// parameters are used, so cost can be raised without invalidating hashes.
func (a *Argon2id) Verify(pw, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, ErrMalformedHash
	}
	var v int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &v); err != nil || v != argon2.Version {
		return false, ErrMalformedHash
	}
	var p Params
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.Memory, &p.Time, &p.Threads); err != nil {
		return false, ErrMalformedHash
	}
	b64 := base64.RawStdEncoding
	salt, err1 := b64.DecodeString(parts[4])
	key, err2 := b64.DecodeString(parts[5])
	if err1 != nil || err2 != nil || len(key) == 0 {
		return false, ErrMalformedHash
	}
	got := argon2.IDKey([]byte(pw), salt, p.Time, p.Memory, p.Threads, uint32(len(key)))
	return subtle.ConstantTimeCompare(got, key) == 1, nil
}
