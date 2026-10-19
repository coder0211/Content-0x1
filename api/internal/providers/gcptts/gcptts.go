// Package gcptts is the voice adapter for Google Cloud Text-to-Speech
// (OQ-01): v1beta1 text:synthesize with SSML <mark> tags before every word so
// the response's timepoints give word timings for subtitles. Usage is billed
// per input character (marks are not billed by Google; we count the plain
// text).
//
// Credentials (secret_ref, default GOOGLE_TTS_CREDENTIALS) may hold either an
// API key or a service-account JSON key; for the latter the adapter signs a
// JWT (RS256) and exchanges it at oauth2.googleapis.com for a cached access
// token. Nothing secret is logged or returned.
package gcptts

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"encoding/pem"
	"html"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/providers/vendorhttp"
)

// Code is ai_providers.code.
const Code = "gcp_tts"

// Defaults.
const (
	DefaultSecretRef = "GOOGLE_TTS_CREDENTIALS" //nolint:gosec // G101: env var name
	DefaultBaseURL   = "https://texttospeech.googleapis.com"
	TokenURL         = "https://oauth2.googleapis.com/token" //nolint:gosec // G101: public endpoint
	scope            = "https://www.googleapis.com/auth/cloud-platform"
	maxInputBytes    = 5000 // API limit for input.ssml
)

// Models seeded in migration 0070 and their voices.
var voicesByModel = map[string][]string{
	"neural2":  {"vi-VN-Neural2-A", "vi-VN-Neural2-D"},
	"wavenet":  {"vi-VN-Wavenet-A", "vi-VN-Wavenet-B", "vi-VN-Wavenet-C", "vi-VN-Wavenet-D"},
	"standard": {"vi-VN-Standard-A", "vi-VN-Standard-B", "vi-VN-Standard-C", "vi-VN-Standard-D"},
}

// DefaultModel is used when the request names none.
const DefaultModel = "neural2"

// Adapter implements ports.VoiceSynthesizer.
type Adapter struct {
	EP *vendorhttp.Endpoint
	// TokenEndpoint is the OAuth token URL (tests override it).
	TokenEndpoint string
	Now           func() time.Time

	mu       sync.Mutex
	token    string
	tokenFor string // client_email the cached token belongs to
	expiry   time.Time
}

var _ ports.VoiceSynthesizer = (*Adapter)(nil)

// New returns an adapter with production defaults.
func New() *Adapter {
	return &Adapter{EP: vendorhttp.NewEndpoint(Code, DefaultSecretRef, DefaultBaseURL, 60*time.Second), TokenEndpoint: TokenURL, Now: time.Now}
}

// Code implements ports.VoiceSynthesizer.
func (*Adapter) Code() string { return Code }

// Configure applies the ai_providers row.
func (a *Adapter) Configure(secretRef string, cfg json.RawMessage) { a.EP.Configure(secretRef, cfg) }

// Capabilities implements ports.VoiceSynthesizer.
func (*Adapter) Capabilities(model string) ports.Capabilities {
	v, ok := voicesByModel[model]
	if !ok {
		v = voicesByModel[DefaultModel]
	}
	return ports.Capabilities{Languages: []string{"vi-VN"}, Voices: v}
}

type synthRequest struct {
	Input struct {
		SSML string `json:"ssml"`
	} `json:"input"`
	Voice struct {
		LanguageCode string `json:"languageCode"`
		Name         string `json:"name"`
	} `json:"voice"`
	AudioConfig struct {
		AudioEncoding   string  `json:"audioEncoding"`
		SpeakingRate    float64 `json:"speakingRate,omitempty"`
		SampleRateHertz int     `json:"sampleRateHertz,omitempty"`
	} `json:"audioConfig"`
	EnableTimePointing []string `json:"enableTimePointing"`
}

type synthResponse struct {
	AudioContent string `json:"audioContent"`
	Timepoints   []struct {
		MarkName    string  `json:"markName"`
		TimeSeconds float64 `json:"timeSeconds"`
	} `json:"timepoints"`
	AudioConfig json.RawMessage `json:"audioConfig"`
}

type googleError struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

func classify(status int, h http.Header, body []byte) *ports.ProviderError {
	var ge googleError
	if json.Unmarshal(body, &ge) != nil || ge.Error.Message == "" {
		return nil //nolint:nilerr // not a vendor error envelope: caller falls back to ByStatus
	}
	code := ge.Error.Status
	if code == "" {
		code = strconv.Itoa(status)
	}
	pe := vendorhttp.ByStatus(Code, status, h, code, ge.Error.Message)
	switch ge.Error.Status {
	case "RESOURCE_EXHAUSTED":
		pe.Kind = ports.ErrKindRateLimited
		if pe.RetryAfter == 0 {
			pe.RetryAfter = 30 * time.Second
		}
	case "UNAUTHENTICATED", "PERMISSION_DENIED":
		pe.Kind = ports.ErrKindAuth
	}
	return pe
}

// BuildSSML escapes text and puts <mark name="wN"/> before each word.
// Returns the SSML and the words in order.
func BuildSSML(text string) (string, []string) {
	words := strings.Fields(text)
	var b strings.Builder
	b.WriteString("<speak>")
	for i, w := range words {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(`<mark name="w` + strconv.Itoa(i) + `"/>`)
		b.WriteString(html.EscapeString(w))
	}
	b.WriteString(`<mark name="end"/></speak>`)
	return b.String(), words
}

// Synthesize implements ports.VoiceSynthesizer.
func (a *Adapter) Synthesize(ctx context.Context, req ports.VoiceRequest) (ports.VoiceResponse, error) {
	model := req.Model
	if model == "" {
		model = DefaultModel
	}
	voices, ok := voicesByModel[model]
	if !ok {
		return ports.VoiceResponse{}, vendorhttp.Unsupported("model_code", model, []string{"neural2", "wavenet", "standard"})
	}
	if err := vendorhttp.CheckOneOf("language", req.Language, []string{"vi-VN"}); err != nil {
		return ports.VoiceResponse{}, err
	}
	voice := req.VoiceID
	if voice == "" {
		voice = voices[0]
	}
	if err := vendorhttp.CheckOneOf("voice_id", voice, voices); err != nil {
		return ports.VoiceResponse{}, err
	}
	format := req.Format
	if format == "" {
		format = "mp3"
	}
	if err := vendorhttp.CheckOneOf("format", format, []string{"mp3", "wav"}); err != nil {
		return ports.VoiceResponse{}, err
	}
	if req.SpeakingRate != 0 && (req.SpeakingRate < 0.25 || req.SpeakingRate > 4) {
		return ports.VoiceResponse{}, vendorhttp.Unsupported("speaking_rate", req.SpeakingRate, map[string]float64{"min": 0.25, "max": 4})
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		return ports.VoiceResponse{}, vendorhttp.Errorf(Code, ports.ErrKindInvalidRequest, "empty_text", "text is required")
	}
	ssml, words := BuildSSML(text)
	if len(ssml) > maxInputBytes {
		return ports.VoiceResponse{}, vendorhttp.Unsupported("text", len([]rune(text)), map[string]int{"max_bytes_ssml": maxInputBytes})
	}
	h, err := a.authHeader(ctx)
	if err != nil {
		return ports.VoiceResponse{}, err
	}
	var body synthRequest
	body.Input.SSML = ssml
	body.Voice.LanguageCode, body.Voice.Name = "vi-VN", voice
	body.AudioConfig.SpeakingRate = req.SpeakingRate
	body.AudioConfig.AudioEncoding = "MP3"
	mime := "audio/mpeg"
	if format == "wav" {
		body.AudioConfig.AudioEncoding, body.AudioConfig.SampleRateHertz, mime = "LINEAR16", 24000, "audio/wav"
	}
	body.EnableTimePointing = []string{"SSML_MARK"}
	var out synthResponse
	if _, err := a.EP.DoJSON(ctx, http.MethodPost, a.EP.BaseURL()+"/v1beta1/text:synthesize", h, body, &out, classify); err != nil {
		return ports.VoiceResponse{}, err
	}
	audio, err := base64.StdEncoding.DecodeString(out.AudioContent)
	if err != nil || len(audio) == 0 {
		return ports.VoiceResponse{}, vendorhttp.Errorf(Code, ports.ErrKindUnknown, "decode_audio", "provider returned no audio")
	}
	var durMs int
	if format == "wav" {
		durMs = WAVDurationMs(audio)
	} else {
		durMs = MP3DurationMs(audio)
	}
	marks := map[string]int{}
	for _, tp := range out.Timepoints {
		marks[tp.MarkName] = int(tp.TimeSeconds * 1000)
	}
	if end, ok := marks["end"]; ok && durMs == 0 {
		durMs = end
	}
	timings := make([]ports.WordTiming, 0, len(words))
	for i, w := range words {
		start, ok := marks["w"+strconv.Itoa(i)]
		if !ok {
			timings = nil // incomplete timepoints: let the caller fall back
			break
		}
		end, ok := marks["w"+strconv.Itoa(i+1)]
		if !ok {
			if end, ok = marks["end"]; !ok {
				end = durMs
			}
		}
		timings = append(timings, ports.WordTiming{Text: w, StartMs: start, EndMs: end})
	}
	chars := len([]rune(text))
	return ports.VoiceResponse{
		MimeType: mime, Audio: audio, DurationMs: durMs, Timings: timings,
		Usage: ports.Usage{Characters: chars, Units: map[string]float64{"char": float64(chars)}},
		Raw: vendorhttp.StripRaw(map[string]any{"voice": voice, "encoding": body.AudioConfig.AudioEncoding,
			"timepoints": len(out.Timepoints), "audio_bytes": len(audio), "audio_config": out.AudioConfig}),
	}, nil
}

// serviceAccount is the subset of a Google service-account key file.
type serviceAccount struct {
	Type        string `json:"type"`
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
}

func (a *Adapter) authHeader(ctx context.Context) (http.Header, error) {
	secret, err := a.EP.Key()
	if err != nil {
		return nil, err
	}
	h := http.Header{}
	if !strings.HasPrefix(secret, "{") {
		h.Set("x-goog-api-key", secret)
		return h, nil
	}
	var sa serviceAccount
	if err := json.Unmarshal([]byte(secret), &sa); err != nil || sa.ClientEmail == "" || sa.PrivateKey == "" {
		return nil, vendorhttp.Errorf(Code, ports.ErrKindAuth, "bad_credentials", "secret %s is not a valid service-account key", a.EP.SecretRef())
	}
	tok, err := a.accessToken(ctx, sa)
	if err != nil {
		return nil, err
	}
	h.Set("Authorization", "Bearer "+tok)
	return h, nil
}

func (a *Adapter) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}

func (a *Adapter) accessToken(ctx context.Context, sa serviceAccount) (string, error) {
	a.mu.Lock()
	if a.token != "" && a.tokenFor == sa.ClientEmail && a.now().Before(a.expiry.Add(-time.Minute)) {
		t := a.token
		a.mu.Unlock()
		return t, nil
	}
	a.mu.Unlock()
	assertion, err := signJWT(sa, a.TokenEndpoint, a.now())
	if err != nil {
		return "", vendorhttp.Errorf(Code, ports.ErrKindAuth, "bad_credentials", "cannot sign token request: %v", err)
	}
	form := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {assertion}}
	resp, err := a.EP.Do(ctx, vendorhttp.Request{Method: http.MethodPost, URL: a.TokenEndpoint, Body: strings.NewReader(form.Encode()),
		ContentType: "application/x-www-form-urlencoded"}, 1<<20, func(status int, h http.Header, body []byte) *ports.ProviderError {
		if status == http.StatusBadRequest || status == http.StatusUnauthorized {
			return &ports.ProviderError{Kind: ports.ErrKindAuth, Provider: Code, Code: "token_" + strconv.Itoa(status), Message: "token exchange rejected"}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	var tr struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(resp.Body, &tr); err != nil || tr.AccessToken == "" {
		return "", vendorhttp.Errorf(Code, ports.ErrKindAuth, "token_decode", "token response without access_token")
	}
	a.mu.Lock()
	a.token, a.tokenFor, a.expiry = tr.AccessToken, sa.ClientEmail, a.now().Add(time.Duration(tr.ExpiresIn)*time.Second)
	a.mu.Unlock()
	return tr.AccessToken, nil
}

func b64url(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func signJWT(sa serviceAccount, aud string, now time.Time) (string, error) {
	block, _ := pem.Decode([]byte(sa.PrivateKey))
	if block == nil {
		return "", errPEM
	}
	var key *rsa.PrivateKey
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rk, ok := k.(*rsa.PrivateKey)
		if !ok {
			return "", errPEM
		}
		key = rk
	} else if rk, err2 := x509.ParsePKCS1PrivateKey(block.Bytes); err2 == nil {
		key = rk
	} else {
		return "", err
	}
	header := b64url([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims, _ := json.Marshal(map[string]any{"iss": sa.ClientEmail, "scope": scope, "aud": aud,
		"iat": now.Unix(), "exp": now.Add(time.Hour).Unix()})
	signing := header + "." + b64url(claims)
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(nil, key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return signing + "." + b64url(sig), nil
}

type pemError struct{}

func (pemError) Error() string { return "private_key is not an RSA PEM key" }

var errPEM error = pemError{}

// WAVDurationMs reads a PCM WAV header; 0 when unparseable.
func WAVDurationMs(b []byte) int {
	if len(b) < 44 || string(b[0:4]) != "RIFF" || string(b[8:12]) != "WAVE" {
		return 0
	}
	var byteRate uint32
	for off := 12; off+8 <= len(b); {
		id := string(b[off : off+4])
		size := binary.LittleEndian.Uint32(b[off+4 : off+8])
		body := off + 8
		switch id {
		case "fmt ":
			if body+12 <= len(b) {
				byteRate = binary.LittleEndian.Uint32(b[body+8 : body+12])
			}
		case "data":
			if byteRate == 0 {
				return 0
			}
			n := int64(size)
			if rest := int64(len(b) - body); n > rest || n == 0xFFFFFFFF {
				n = rest
			}
			return int(n * 1000 / int64(byteRate))
		}
		off = body + int(size) + int(size&1)
	}
	return 0
}

var mp3Bitrates = [2][16]int{
	{0, 32, 40, 48, 56, 64, 80, 96, 112, 128, 160, 192, 224, 256, 320, 0}, // MPEG1 L3
	{0, 8, 16, 24, 32, 40, 48, 56, 64, 80, 96, 112, 128, 144, 160, 0},     // MPEG2/2.5 L3
}

var mp3Rates = map[int][3]int{3: {44100, 48000, 32000}, 2: {22050, 24000, 16000}, 0: {11025, 12000, 8000}}

// MP3DurationMs walks MPEG layer III frame headers (skipping an ID3v2 tag)
// and sums frame durations; 0 when no frame is found. The authoritative
// duration is measured with ffprobe by the caller (ACS-062 AC).
func MP3DurationMs(b []byte) int {
	off := 0
	if len(b) >= 10 && bytes.Equal(b[:3], []byte("ID3")) {
		size := int(b[6]&0x7f)<<21 | int(b[7]&0x7f)<<14 | int(b[8]&0x7f)<<7 | int(b[9]&0x7f)
		off = 10 + size
	}
	var samples, rate int
	for off+4 <= len(b) {
		if b[off] != 0xFF || b[off+1]&0xE0 != 0xE0 {
			off++
			continue
		}
		version := int(b[off+1]>>3) & 3 // 3=MPEG1, 2=MPEG2, 0=MPEG2.5
		layer := int(b[off+1]>>1) & 3   // 1 = layer III
		bi := int(b[off+2] >> 4)
		ri := int(b[off+2]>>2) & 3
		pad := int(b[off+2]>>1) & 1
		rates, ok := mp3Rates[version]
		if layer != 1 || !ok || ri == 3 || bi == 0 || bi == 15 {
			off++
			continue
		}
		sr := rates[ri]
		var frameLen, spf int
		if version == 3 {
			spf = 1152
			frameLen = 144*mp3Bitrates[0][bi]*1000/sr + pad
		} else {
			spf = 576
			frameLen = 72*mp3Bitrates[1][bi]*1000/sr + pad
		}
		if frameLen < 4 {
			off++
			continue
		}
		samples += spf
		rate = sr
		off += frameLen
	}
	if rate == 0 {
		return 0
	}
	return samples * 1000 / rate
}
