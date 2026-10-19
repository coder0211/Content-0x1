package usecase

import (
	"strings"
	"testing"
)

// ACS-101: the extension follows the verified MIME type and the name is
// capped at 80 characters.
func TestDownloadName(t *testing.T) {
	cases := []struct{ title, original, mime, want string }{
		{"Ưu đãi hè", "poster.html", "image/png", "Uu-dai-he.png"},
		{"Bài viết", "export.txt", "text/plain", "Bai-viet.txt"},
		{"", "clip.MOV", "video/mp4", "asset.mp4"},
		{"x", "data.bin1", "application/x-unknown-acs", "x.bin1"},
		{"x", "evil.ht\"ml", "application/x-unknown-acs", "x"},
	}
	for _, c := range cases {
		if got := downloadName(c.title, c.original, c.mime); got != c.want {
			t.Errorf("downloadName(%q, %q, %q) = %q, want %q", c.title, c.original, c.mime, got, c.want)
		}
	}
	if got := downloadName(strings.Repeat("a", 300), "", "image/png"); len(got) != 80+len(".png") {
		t.Errorf("name not capped: %d", len(got))
	}
}
