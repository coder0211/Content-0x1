package domain

import (
	"bytes"
	"unicode/utf8"
)

// SniffLen is how many leading bytes Sniff needs.
const SniffLen = 512

// Sniff detects the real MIME type from magic bytes; "" when unknown
// (e.g. an executable renamed to .png).
func Sniff(b []byte) string {
	switch {
	case bytes.HasPrefix(b, []byte("\x89PNG\r\n\x1a\n")):
		return mimePNG
	case bytes.HasPrefix(b, []byte{0xFF, 0xD8, 0xFF}):
		return mimeJPEG
	case len(b) >= 12 && bytes.Equal(b[0:4], []byte("RIFF")) && bytes.Equal(b[8:12], []byte("WEBP")):
		return mimeWEBP
	case len(b) >= 12 && bytes.Equal(b[0:4], []byte("RIFF")) && bytes.Equal(b[8:12], []byte("WAVE")):
		return mimeWAV
	case bytes.HasPrefix(b, []byte("%PDF-")):
		return mimePDF
	case bytes.HasPrefix(b, []byte{0x1A, 0x45, 0xDF, 0xA3}):
		return mimeWEBM
	case len(b) >= 12 && bytes.Equal(b[4:8], []byte("ftyp")):
		brand := string(b[8:12])
		switch brand {
		case "qt  ":
			return mimeMOV
		case "M4A ", "M4B ":
			return mimeM4A
		}
		return mimeMP4
	case bytes.HasPrefix(b, []byte("ID3")), len(b) >= 2 && b[0] == 0xFF && (b[1]&0xE0) == 0xE0:
		return mimeMP3
	case isSVG(b):
		return mimeSVG
	case isText(b):
		return mimeTXT
	}
	return ""
}

func isSVG(b []byte) bool {
	t := bytes.TrimSpace(bytes.TrimPrefix(b, []byte("\xef\xbb\xbf")))
	if !bytes.HasPrefix(t, []byte("<?xml")) && !bytes.HasPrefix(t, []byte("<svg")) {
		return false
	}
	return bytes.Contains(bytes.ToLower(b), []byte("<svg"))
}

func isText(b []byte) bool {
	if len(b) == 0 || bytes.IndexByte(b, 0) >= 0 {
		return false
	}
	// Tolerate a multi-byte rune cut at the sniff boundary.
	for i := 0; i < 4 && !utf8.Valid(b); i++ {
		b = b[:len(b)-1]
	}
	return utf8.Valid(b)
}

// SniffMatches reports whether sniffed content is acceptable for the
// declared MIME type. ISO-BMFF brands (mp4/mov/m4a) are interchangeable here;
// ffprobe then verifies the actual streams.
func SniffMatches(declared, sniffed string) bool {
	if declared == sniffed {
		return true
	}
	isoBMFF := map[string]bool{mimeMP4: true, mimeMOV: true, mimeM4A: true}
	return isoBMFF[declared] && isoBMFF[sniffed]
}
