package domain

import (
	"errors"
	"testing"

	"github.com/be/acs/api/internal/shared/kernel"
)

var limits = Limits{Image: 20 << 20, Video: 500 << 20, Audio: 50 << 20, Document: 20 << 20}

func TestSniff(t *testing.T) {
	cases := map[string][]byte{
		mimePNG:  []byte("\x89PNG\r\n\x1a\n\x00\x00"),
		mimeJPEG: {0xFF, 0xD8, 0xFF, 0xE0},
		mimeWEBP: []byte("RIFF\x00\x00\x00\x00WEBPVP8 "),
		mimeWAV:  []byte("RIFF\x00\x00\x00\x00WAVEfmt "),
		mimeMP4:  []byte("\x00\x00\x00\x18ftypisom\x00\x00"),
		mimeMOV:  []byte("\x00\x00\x00\x14ftypqt  \x00\x00"),
		mimeM4A:  []byte("\x00\x00\x00\x18ftypM4A \x00\x00"),
		mimeMP3:  []byte("ID3\x03\x00"),
		mimePDF:  []byte("%PDF-1.7"),
		mimeSVG:  []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"/>`),
		mimeTXT:  []byte("Xin chào, đây là tài liệu."),
		"":       []byte("MZ\x90\x00\x03\x00\x00\x00\x04\x00"), // Windows executable
	}
	for want, b := range cases {
		if got := Sniff(b); got != want {
			t.Errorf("Sniff(%q) = %q, want %q", b[:4], got, want)
		}
	}
}

func TestValidateUploadRequest(t *testing.T) {
	if err := ValidateUploadRequest(PurposeReferenceImage, "a.png", "image/png", 1024, limits); err != nil {
		t.Fatal(err)
	}
	if err := ValidateUploadRequest(PurposeBrandLogo, "logo.png", "image/png", 6<<20, limits); !errors.Is(err, kernel.ErrFileTooLarge) {
		t.Fatalf("logo > 5MB: %v", err)
	}
	if err := ValidateUploadRequest(PurposeReferenceImage, "a.exe", "application/x-msdownload", 10, limits); !errors.Is(err, kernel.ErrUnsupportedMedia) {
		t.Fatalf("exe: %v", err)
	}
	if err := ValidateUploadRequest(PurposeVideoSource, "clip.mp4", "video/mp4", 501<<20, limits); !errors.Is(err, kernel.ErrFileTooLarge) {
		t.Fatalf("video > 500MB: %v", err)
	}
	if err := ValidateUploadRequest("nope", "a", "image/png", 1, limits); !errors.Is(err, kernel.ErrValidation) {
		t.Fatalf("purpose: %v", err)
	}
}
