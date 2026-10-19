// Package config loads process configuration from environment variables.
// Every setting has a development default matching docker-compose.yml, so
// `make run-api` works on a fresh checkout; production must override secrets.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the full process configuration.
type Config struct {
	Env        string // local | staging | production
	HTTPAddr   string
	AppBaseURL string
	LogLevel   string

	DatabaseURL string
	RedisURL    string

	S3      S3Config
	Session SessionConfig
	Upload  UploadConfig
	Worker  WorkerConfig
	SMTP    SMTPConfig
	FFmpeg  FFmpegConfig

	CORSOrigins  []string
	SignedURLTTL time.Duration
	// EnableFakeProviders registers the fake AI adapters (always on outside production).
	EnableFakeProviders bool
}

// S3Config configures object storage. PublicEndpoint is the host browsers
// use for presigned URLs when it differs from the internal endpoint (MinIO
// in docker-compose: minio:9000 internally, localhost:9000 for the browser).
type S3Config struct {
	Endpoint       string
	PublicEndpoint string
	Region         string
	Bucket         string
	AccessKey      string
	SecretKey      string
	UseSSL         bool
}

// SessionConfig configures cookie sessions (SPEC-02 §7).
type SessionConfig struct {
	CookieName   string
	CSRFCookie   string
	CookieSecure bool
	CookieDomain string
	IdleTimeout  time.Duration
	Absolute     time.Duration
	MaxFailed    int
	LockoutFor   time.Duration
}

// UploadConfig caps upload sizes (NFR-07).
type UploadConfig struct {
	MaxImageBytes    int64
	MaxVideoBytes    int64
	MaxAudioBytes    int64
	MaxDocumentBytes int64
	PendingTTL       time.Duration
}

// WorkerConfig configures the worker process.
type WorkerConfig struct {
	Queues      []string
	Concurrency int
	ID          string
}

// SMTPConfig configures outgoing mail (Mailpit locally).
type SMTPConfig struct {
	Host string
	Port int
	From string
}

// FFmpegConfig locates the binaries and scratch space.
type FFmpegConfig struct {
	FFmpegPath  string
	FFprobePath string
	TempDir     string
	// FontsDir is passed to libass for subtitle burn-in (fonts with
	// Vietnamese glyphs, e.g. /usr/share/fonts/noto in the render image).
	// Empty = libass/fontconfig system defaults.
	FontsDir string
}

// IsProduction reports whether Env is production.
func (c Config) IsProduction() bool { return c.Env == "production" }

// Load reads configuration from the environment.
func Load() (Config, error) {
	return load(os.Getenv)
}

func load(getenv func(string) string) (Config, error) {
	e := env{get: getenv}
	host, _ := os.Hostname()
	c := Config{
		Env:         e.str("APP_ENV", "local"),
		HTTPAddr:    e.str("HTTP_ADDR", ":8080"),
		AppBaseURL:  e.str("APP_BASE_URL", "http://localhost:5173"),
		LogLevel:    e.str("LOG_LEVEL", "info"),
		DatabaseURL: e.str("DATABASE_URL", "postgres://acs:acs@localhost:5432/acs?sslmode=disable"),
		RedisURL:    e.str("REDIS_URL", "redis://localhost:6379/0"),
		S3: S3Config{
			Endpoint:       e.str("S3_ENDPOINT", "localhost:9000"),
			PublicEndpoint: e.str("S3_PUBLIC_ENDPOINT", ""),
			Region:         e.str("S3_REGION", "us-east-1"),
			Bucket:         e.str("S3_BUCKET", "acs-assets"),
			AccessKey:      e.str("S3_ACCESS_KEY", "minioadmin"),
			SecretKey:      e.str("S3_SECRET_KEY", "minioadmin"),
			UseSSL:         e.boolean("S3_USE_SSL", false),
		},
		Session: SessionConfig{
			CookieName:   e.str("SESSION_COOKIE", "acs_session"),
			CSRFCookie:   e.str("CSRF_COOKIE", "acs_csrf"),
			CookieSecure: e.boolean("COOKIE_SECURE", false),
			CookieDomain: e.str("COOKIE_DOMAIN", ""),
			IdleTimeout:  e.duration("SESSION_IDLE_TIMEOUT", 12*time.Hour),
			Absolute:     e.duration("SESSION_ABSOLUTE_TIMEOUT", 7*24*time.Hour),
			MaxFailed:    e.integer("LOGIN_MAX_FAILED", 5),
			LockoutFor:   e.duration("LOGIN_LOCKOUT", 15*time.Minute),
		},
		Upload: UploadConfig{
			MaxImageBytes:    e.int64("UPLOAD_MAX_IMAGE_BYTES", 20<<20),
			MaxVideoBytes:    e.int64("UPLOAD_MAX_VIDEO_BYTES", 500<<20),
			MaxAudioBytes:    e.int64("UPLOAD_MAX_AUDIO_BYTES", 50<<20),
			MaxDocumentBytes: e.int64("UPLOAD_MAX_DOCUMENT_BYTES", 20<<20),
			PendingTTL:       e.duration("UPLOAD_PENDING_TTL", 24*time.Hour),
		},
		Worker: WorkerConfig{
			Queues:      e.list("WORKER_QUEUES", []string{"text", "image", "video", "render", "maintenance"}),
			Concurrency: e.integer("WORKER_CONCURRENCY", 20),
			ID:          e.str("WORKER_ID", host),
		},
		SMTP: SMTPConfig{
			Host: e.str("SMTP_HOST", "localhost"),
			Port: e.integer("SMTP_PORT", 1025),
			From: e.str("SMTP_FROM", "AI Content Studio <no-reply@acs.local>"),
		},
		FFmpeg: FFmpegConfig{
			FFmpegPath:  e.str("FFMPEG_PATH", "ffmpeg"),
			FFprobePath: e.str("FFPROBE_PATH", "ffprobe"),
			TempDir:     e.str("MEDIA_TEMP_DIR", os.TempDir()),
			FontsDir:    e.str("VIDEO_FONTS_DIR", ""),
		},
		CORSOrigins:  e.list("CORS_ORIGINS", []string{"http://localhost:5173"}),
		SignedURLTTL: e.duration("SIGNED_URL_TTL", 15*time.Minute),
	}
	c.EnableFakeProviders = e.boolean("ENABLE_FAKE_PROVIDERS", !c.IsProduction())
	if len(e.errs) > 0 {
		return Config{}, errors.Join(e.errs...)
	}
	return c, c.validate()
}

func (c Config) validate() error {
	var errs []error
	if c.SignedURLTTL <= 0 || c.SignedURLTTL > 15*time.Minute {
		errs = append(errs, errors.New("SIGNED_URL_TTL must be in (0, 15m] (FR-IMG-05)"))
	}
	if c.IsProduction() {
		if !c.Session.CookieSecure {
			errs = append(errs, errors.New("COOKIE_SECURE must be true in production"))
		}
		if c.S3.AccessKey == "minioadmin" {
			errs = append(errs, errors.New("S3 default credentials are not allowed in production"))
		}
		if c.EnableFakeProviders {
			errs = append(errs, errors.New("ENABLE_FAKE_PROVIDERS must be false in production"))
		}
	}
	return errors.Join(errs...)
}

type env struct {
	get  func(string) string
	errs []error
}

func (e *env) str(key, def string) string {
	if v := strings.TrimSpace(e.get(key)); v != "" {
		return v
	}
	return def
}

func (e *env) integer(key string, def int) int {
	v := e.get(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		e.errs = append(e.errs, fmt.Errorf("%s: %w", key, err))
	}
	return n
}

func (e *env) int64(key string, def int64) int64 {
	v := e.get(key)
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		e.errs = append(e.errs, fmt.Errorf("%s: %w", key, err))
	}
	return n
}

func (e *env) boolean(key string, def bool) bool {
	v := e.get(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		e.errs = append(e.errs, fmt.Errorf("%s: %w", key, err))
	}
	return b
}

func (e *env) duration(key string, def time.Duration) time.Duration {
	v := e.get(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		e.errs = append(e.errs, fmt.Errorf("%s: %w", key, err))
	}
	return d
}

func (e *env) list(key string, def []string) []string {
	v := e.get(key)
	if v == "" {
		return def
	}
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Secret returns the value of the environment variable named by ref
// (ai_providers.secret_ref). It never logs or caches the value.
//
// Only provider-credential names resolve (IsProviderSecretRef): an Admin
// editing ai_providers.secret_ref must not be able to point a vendor
// adapter at a platform secret such as DATABASE_URL or S3_SECRET_KEY and
// send it to the vendor base_url (ACS-101).
func Secret(ref string) string {
	if !IsProviderSecretRef(ref) {
		return ""
	}
	return os.Getenv(ref)
}

// providerSecretSuffixes are the accepted endings of a provider secret name.
var providerSecretSuffixes = []string{"_API_KEY", "_CREDENTIALS", "_TOKEN"}

// platformSecretPrefixes name platform configuration that must never be
// handed to a vendor adapter.
var platformSecretPrefixes = []string{"DATABASE_", "REDIS_", "S3_", "SMTP_", "SESSION_", "COOKIE_", "CSRF_", "APP_", "ACS_"}

// IsProviderSecretRef reports whether ref may name a provider credential:
// an upper-case env var name ending in _API_KEY, _CREDENTIALS or _TOKEN
// that is not platform configuration. "none" and "" are not references.
func IsProviderSecretRef(ref string) bool {
	if ref == "" || ref == "none" || len(ref) > 64 {
		return false
	}
	for _, r := range ref {
		if (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' {
			return false
		}
	}
	for _, p := range platformSecretPrefixes {
		if strings.HasPrefix(ref, p) {
			return false
		}
	}
	for _, s := range providerSecretSuffixes {
		if strings.HasSuffix(ref, s) && len(ref) > len(s) {
			return true
		}
	}
	return false
}
