// Package domain holds file rules: allowed upload purposes, MIME types,
// size limits and magic-byte sniffing (NFR-07).
package domain

import (
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/be/acs/api/internal/shared/kernel"
)

// Kind mirrors asset_files.kind.
type Kind string

const (
	KindImage    Kind = "image"
	KindVideo    Kind = "video"
	KindAudio    Kind = "audio"
	KindSubtitle Kind = "subtitle"
	KindDocument Kind = "document"
	KindOther    Kind = "other"
)

// UploadStatus mirrors asset_files.upload_status.
type UploadStatus string

const (
	StatusPending  UploadStatus = "pending"
	StatusReady    UploadStatus = "ready"
	StatusRejected UploadStatus = "rejected"
)

// Source mirrors asset_files.source.
type Source string

const (
	SourceUpload    Source = "upload"
	SourceGenerated Source = "generated"
	SourceRendered  Source = "rendered"
)

// File is an object in storage plus its metadata.
type File struct {
	ID             kernel.ID
	StorageKey     string
	OriginalName   string
	MimeType       string
	SizeBytes      int64
	ChecksumSHA256 string
	Width          int
	Height         int
	DurationMs     int
	Kind           Kind
	UploadStatus   UploadStatus
	Source         Source
	ParentFileID   *kernel.ID
	License        json.RawMessage
	Metadata       json.RawMessage
	CreatedBy      *kernel.ID
	CreatedAt      time.Time
}

// Purpose is why a browser uploads a file; it bounds MIME types and size.
type Purpose string

const (
	PurposeBrandLogo      Purpose = "brand_logo"      // PNG/SVG/JPG ≤ 5MB (FR-BRD-01)
	PurposeReferenceImage Purpose = "reference_image" // brand/library/image-to-image refs
	PurposeImageMask      Purpose = "image_mask"
	PurposeVideoSource    Purpose = "video_source" // images or clips for Video Studio
	PurposeMusic          Purpose = "music"        // licensed background music
	PurposeDocument       Purpose = "document"     // brand reference documents
)

// Limits are per-kind maximum sizes (from config).
type Limits struct {
	Image    int64
	Video    int64
	Audio    int64
	Document int64
}

// Rule is the policy for one purpose.
type Rule struct {
	MimeTypes []string
	MaxBytes  func(Limits) int64
}

const (
	mimePNG  = "image/png"
	mimeJPEG = "image/jpeg"
	mimeWEBP = "image/webp"
	mimeSVG  = "image/svg+xml"
	mimeMP4  = "video/mp4"
	mimeMOV  = "video/quicktime"
	mimeWEBM = "video/webm"
	mimeMP3  = "audio/mpeg"
	mimeWAV  = "audio/wav"
	mimeM4A  = "audio/mp4"
	mimePDF  = "application/pdf"
	mimeTXT  = "text/plain"
)

var rules = map[Purpose]Rule{
	PurposeBrandLogo:      {[]string{mimePNG, mimeJPEG, mimeSVG}, func(Limits) int64 { return 5 << 20 }},
	PurposeReferenceImage: {[]string{mimePNG, mimeJPEG, mimeWEBP}, func(l Limits) int64 { return l.Image }},
	PurposeImageMask:      {[]string{mimePNG}, func(l Limits) int64 { return l.Image }},
	PurposeVideoSource:    {[]string{mimePNG, mimeJPEG, mimeWEBP, mimeMP4, mimeMOV, mimeWEBM}, nil},
	PurposeMusic:          {[]string{mimeMP3, mimeWAV, mimeM4A}, func(l Limits) int64 { return l.Audio }},
	PurposeDocument:       {[]string{mimePDF, mimeTXT}, func(l Limits) int64 { return l.Document }},
}

// KindOf maps a MIME type to a file kind.
func KindOf(mime string) Kind {
	switch {
	case strings.HasPrefix(mime, "image/"):
		return KindImage
	case strings.HasPrefix(mime, "video/"):
		return KindVideo
	case strings.HasPrefix(mime, "audio/"):
		return KindAudio
	case mime == "application/x-subrip" || mime == "text/x-ssa":
		return KindSubtitle
	case mime == mimePDF || mime == mimeTXT:
		return KindDocument
	}
	return KindOther
}

// MaxBytesFor returns the size cap for mime under purpose.
func MaxBytesFor(p Purpose, mime string, l Limits) int64 {
	r := rules[p]
	if r.MaxBytes != nil {
		return r.MaxBytes(l)
	}
	switch KindOf(mime) {
	case KindVideo:
		return l.Video
	case KindAudio:
		return l.Audio
	case KindImage:
		return l.Image
	}
	return l.Document
}

// ValidateUploadRequest checks a requested upload before presigning.
func ValidateUploadRequest(p Purpose, filename, mime string, size int64, l Limits) error {
	r, ok := rules[p]
	if !ok {
		return kernel.Validation(kernel.FieldIssue("purpose", "invalid"))
	}
	var v kernel.ValidationErrors
	if strings.TrimSpace(filename) == "" || len(filename) > 255 {
		v.Add("filename", "invalid")
	}
	if size <= 0 {
		v.Add("size_bytes", "invalid")
	}
	if err := v.Err(); err != nil {
		return err
	}
	if !slices.Contains(r.MimeTypes, mime) {
		return kernel.ErrUnsupportedMedia.WithDetails(kernel.Detail{"field": "mime_type", "supported": r.MimeTypes})
	}
	if max := MaxBytesFor(p, mime, l); size > max {
		return kernel.ErrFileTooLarge.WithDetails(kernel.Detail{"field": "size_bytes", "max": max})
	}
	return nil
}

// IsActiveContent reports MIME types a browser would execute as a document
// (script-capable) when opened directly from the storage origin.
func IsActiveContent(mime string) bool {
	switch mime {
	case "image/svg+xml", "text/html", "application/xhtml+xml", "text/xml", "application/xml":
		return true
	}
	return false
}

// ExtensionFor returns a file extension for the MIME types served here.
func ExtensionFor(mime string) string {
	switch mime {
	case "image/svg+xml":
		return ".svg"
	case "text/html", "application/xhtml+xml":
		return ".html"
	case "text/xml", "application/xml":
		return ".xml"
	}
	return ""
}
