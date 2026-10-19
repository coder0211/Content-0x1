package domain

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/be/acs/api/internal/shared/kernel"
)

// PlanItem is one planned output (FR-CMP-03: topic, angle, publish date,
// output list). Done is a checklist flag for the Overview tab.
type PlanItem struct {
	ID          string `json:"id"`
	Topic       string `json:"topic"`
	Angle       string `json:"angle"`
	Channel     string `json:"channel"`
	Format      string `json:"format"`
	PublishDate string `json:"publish_date"`
	Notes       string `json:"notes"`
	Done        bool   `json:"done"`
}

// Plan is campaigns.content_plan (and content_plan_proposal).
//
//	{"summary": "...", "themes": ["..."], "items": [PlanItem...]}
type Plan struct {
	Summary string     `json:"summary"`
	Themes  []string   `json:"themes"`
	Items   []PlanItem `json:"items"`
}

// Normalize trims text, fills missing item ids and replaces nil slices.
func (p Plan) Normalize() Plan {
	p.Summary = strings.TrimSpace(p.Summary)
	themes := make([]string, 0, len(p.Themes))
	for _, t := range p.Themes {
		if t = strings.TrimSpace(t); t != "" {
			themes = append(themes, t)
		}
	}
	p.Themes = themes
	items := make([]PlanItem, 0, len(p.Items))
	for i, it := range p.Items {
		it.ID = strings.TrimSpace(it.ID)
		if it.ID == "" {
			it.ID = fmt.Sprintf("item-%d", i+1)
		}
		it.Topic = strings.TrimSpace(it.Topic)
		it.Angle = strings.TrimSpace(it.Angle)
		it.Channel = strings.ToLower(strings.TrimSpace(it.Channel))
		it.Format = strings.ToLower(strings.TrimSpace(it.Format))
		it.PublishDate = strings.TrimSpace(it.PublishDate)
		it.Notes = strings.TrimSpace(it.Notes)
		items = append(items, it)
	}
	p.Items = items
	return p
}

// Validate reports problems of a normalized plan under prefix ("plan").
func (p Plan) Validate(v *kernel.ValidationErrors, prefix string) {
	if tooLong(p.Summary, MaxNotesLen) {
		v.Add(prefix+".summary", "too_long")
	}
	if len(p.Themes) > MaxListItems {
		v.Add(prefix+".themes", "too_many")
	}
	switch {
	case len(p.Items) == 0:
		v.Add(prefix+".items", "required")
	case len(p.Items) > MaxPlanItems:
		v.Add(prefix+".items", "too_many")
	}
	ids := map[string]bool{}
	for i, it := range p.Items {
		f := fmt.Sprintf("%s.items[%d]", prefix, i)
		if ids[it.ID] {
			v.Add(f+".id", "duplicate")
		}
		ids[it.ID] = true
		if it.Topic == "" {
			v.Add(f+".topic", "required")
		}
		for _, t := range [][2]string{{"topic", it.Topic}, {"angle", it.Angle}, {"notes", it.Notes}} {
			if tooLong(t[1], MaxPlanText) {
				v.Add(f+"."+t[0], "too_long")
			}
		}
		if it.Format == "" {
			v.Add(f+".format", "required")
		} else if !slug.MatchString(it.Format) {
			v.Add(f+".format", "invalid")
		}
		if it.Channel != "" && !slug.MatchString(it.Channel) {
			v.Add(f+".channel", "invalid")
		}
		if it.PublishDate != "" && !ValidDate(it.PublishDate) {
			v.Add(f+".publish_date", "invalid_date")
		}
	}
}

// ParsePlan decodes a stored or AI-produced plan tolerantly; nil for empty.
func ParsePlan(raw json.RawMessage) (*Plan, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var p Plan
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	n := p.Normalize()
	return &n, nil
}
