package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExportText(t *testing.T) {
	cases := map[string]string{
		`{"title":"Tiêu đề","hook":"Hook","body_text":"Thân bài","cta":"Đăng ký","hashtags":["tienganh","#hoc"]}`: "Tiêu đề\n\nHook\n\nThân bài\n\nĐăng ký\n\n#tienganh #hoc\n",
		`{"title":"T","body_html":"<p>Một</p><p>Hai</p>"}`:                                                        "T\n\nMột  Hai\n",
		`{"prompt":"ảnh"}`: "",
		`not json`:         "",
	}
	for in, want := range cases {
		if got := ExportText([]byte(in)); got != want {
			t.Errorf("ExportText(%s) = %q, want %q", in, got, want)
		}
	}
}

func TestSanitizeContentBody(t *testing.T) {
	decode := func(raw []byte) map[string]string {
		var m map[string]string
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	m := decode(SanitizeContentBody([]byte(`{"title":"T","body_text":"Một <b>\n\nHai","body_html":"<img src=x onerror=alert(1)>"}`)))
	if m["body_html"] != `<p>Một &lt;b&gt;</p><p>Hai</p>` || m["title"] != "T" {
		t.Fatalf("got %+v", m)
	}
	m = decode(SanitizeContentBody([]byte(`{"body_html":"<p>A</p><script>x</script>"}`)))
	if strings.Contains(m["body_html"], "<script>") {
		t.Fatalf("got %+v", m)
	}
	if got := string(SanitizeContentBody([]byte(`{"prompt":"x"}`))); got != `{"prompt":"x"}` {
		t.Fatalf("untouched content changed: %s", got)
	}
}
