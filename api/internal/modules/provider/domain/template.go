package domain

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"

	"github.com/be/acs/api/internal/shared/kernel"
)

// TemplateStatus is prompt_templates.status.
type TemplateStatus string

const (
	TemplateDraft   TemplateStatus = "draft"
	TemplateActive  TemplateStatus = "active"
	TemplateRetired TemplateStatus = "retired"
)

// PromptTemplate is one immutable version of a prompt (FR-CNT-08). Only the
// status changes after creation.
type PromptTemplate struct {
	ID           kernel.ID
	Code         string
	Version      int
	SystemPrompt string
	UserTemplate string
	OutputSchema json.RawMessage // nil when the prompt has no structured output
	Status       TemplateStatus
	CreatedBy    *kernel.ID
	CreatedAt    time.Time
}

var templateCodeRe = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)

const maxPromptLen = 50_000

// NewTemplateVersion validates and builds version n of code.
func NewTemplateVersion(code string, version int, system, user string, schema json.RawMessage, createdBy kernel.ID, now time.Time) (*PromptTemplate, error) {
	var v kernel.ValidationErrors
	code = strings.TrimSpace(code)
	if !templateCodeRe.MatchString(code) || len(code) > 80 {
		v.AddDetail(kernel.Detail{"field": "code", "issue": "invalid_format", "hint": "vd. content.facebook_post"})
	}
	if strings.TrimSpace(system) == "" {
		v.Add("system_prompt", "required")
	} else if len(system) > maxPromptLen {
		v.Add("system_prompt", "too_long")
	}
	if strings.TrimSpace(user) == "" {
		v.Add("user_template", "required")
	} else if len(user) > maxPromptLen {
		v.Add("user_template", "too_long")
	}
	if len(schema) > 0 && string(schema) != "null" {
		var m map[string]any
		if err := json.Unmarshal(schema, &m); err != nil || m == nil {
			v.Add("output_schema", "must_be_object")
		} else if len(schema) > 64<<10 {
			v.Add("output_schema", "too_large")
		}
	} else {
		schema = nil
	}
	if err := v.Err(); err != nil {
		return nil, err
	}
	by := createdBy
	return &PromptTemplate{ID: kernel.NewID(), Code: code, Version: version, SystemPrompt: system, UserTemplate: user,
		OutputSchema: schema, Status: TemplateDraft, CreatedBy: &by, CreatedAt: now}, nil
}

// Activate makes a draft or retired version active (rollback is allowed).
func (t *PromptTemplate) Activate() error {
	if t.Status == TemplateActive {
		return kernel.InvalidTransition(string(t.Status), string(TemplateActive))
	}
	t.Status = TemplateActive
	return nil
}

// Retire takes a draft or active version out of use.
func (t *PromptTemplate) Retire() error {
	if t.Status == TemplateRetired {
		return kernel.InvalidTransition(string(t.Status), string(TemplateRetired))
	}
	t.Status = TemplateRetired
	return nil
}
