package domain

import (
	"encoding/json"
	"html"
	"strings"
)

// ExportText renders a content version (migration 0006 schema) as a plain
// UTF-8 text document for download: title, hook, body, CTA and hashtags,
// separated by blank lines. body_html is used (tags stripped) only when no
// body_text exists. Returns "" when there is nothing to export.
func ExportText(content json.RawMessage) string {
	var m map[string]any
	if json.Unmarshal(content, &m) != nil {
		return ""
	}
	str := func(k string) string {
		s, _ := m[k].(string)
		return strings.TrimSpace(s)
	}
	body := str("body_text")
	if body == "" {
		body = strings.TrimSpace(stripTags(str("body_html")))
	}
	var parts []string
	for _, p := range []string{str("title"), str("hook"), body, str("cta")} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	if hs, ok := m["hashtags"].([]any); ok {
		var tags []string
		for _, h := range hs {
			if s, ok := h.(string); ok && strings.TrimSpace(s) != "" {
				s = strings.TrimSpace(s)
				if !strings.HasPrefix(s, "#") {
					s = "#" + s
				}
				tags = append(tags, s)
			}
		}
		if len(tags) > 0 {
			parts = append(parts, strings.Join(tags, " "))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n\n") + "\n"
}

// SanitizeContentBody makes body_html of a content version safe: when the
// version has body_text, body_html is regenerated from it (escaped
// paragraphs, the same markup the content module renders); otherwise a
// client body_html is reduced to escaped text. Other keys are untouched.
// Invalid JSON is returned unchanged (validated elsewhere).
func SanitizeContentBody(content json.RawMessage) json.RawMessage {
	var m map[string]json.RawMessage
	if json.Unmarshal(content, &m) != nil {
		return content
	}
	var text, rawHTML string
	_ = json.Unmarshal(m["body_text"], &text)
	_ = json.Unmarshal(m["body_html"], &rawHTML)
	if _, ok := m["body_html"]; !ok && text == "" {
		return content
	}
	if text == "" {
		text = strings.TrimSpace(stripTags(rawHTML))
	}
	b, err := json.Marshal(RenderBodyHTML(text))
	if err != nil {
		return content
	}
	m["body_html"] = b
	out, err := json.Marshal(m)
	if err != nil {
		return content
	}
	return out
}

// RenderBodyHTML renders plain text as escaped <p> paragraphs with <br>
// line breaks (mirrors content/domain.RenderHTML).
func RenderBodyHTML(text string) string {
	text = strings.ReplaceAll(strings.TrimSpace(text), "\r\n", "\n")
	if text == "" {
		return ""
	}
	var b strings.Builder
	for _, para := range strings.Split(text, "\n\n") {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		lines := strings.Split(para, "\n")
		for i := range lines {
			lines[i] = html.EscapeString(strings.TrimSpace(lines[i]))
		}
		b.WriteString("<p>")
		b.WriteString(strings.Join(lines, "<br>"))
		b.WriteString("</p>")
	}
	return b.String()
}
