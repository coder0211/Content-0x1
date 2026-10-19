// Package domain holds the campaign entity, brief validation, the status
// state machine (SPEC-03 §2) and the content plan.
package domain

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/be/acs/api/internal/shared/kernel"
)

// Status mirrors campaigns.status.
type Status string

// Campaign statuses.
const (
	StatusDraft      Status = "draft"
	StatusInProgress Status = "in_progress"
	StatusReview     Status = "review"
	StatusCompleted  Status = "completed"
	StatusArchived   Status = "archived"
)

// Statuses lists every status in lifecycle order.
var Statuses = []Status{StatusDraft, StatusInProgress, StatusReview, StatusCompleted, StatusArchived}

// IsStatus reports whether s is a known status.
func IsStatus(s string) bool { return slices.Contains(Statuses, Status(s)) }

// transitions is SPEC-03 §2: draft → in_progress → review → completed →
// archived, in_progress ↔ review, any state → archived.
var transitions = map[Status][]Status{
	StatusDraft:      {StatusInProgress, StatusArchived},
	StatusInProgress: {StatusReview, StatusArchived},
	StatusReview:     {StatusInProgress, StatusCompleted, StatusArchived},
	StatusCompleted:  {StatusArchived},
	StatusArchived:   {},
}

// CanTransition reports whether from → to is allowed.
func CanTransition(from, to Status) bool { return slices.Contains(transitions[from], to) }

// NextStatuses lists the statuses reachable from s.
func NextStatuses(s Status) []Status { return slices.Clone(transitions[s]) }

// AcceptsGeneration reports whether new AI work may be created in s.
func (s Status) AcceptsGeneration() bool { return s != StatusCompleted && s != StatusArchived }

// Goals are campaigns.goal values.
var Goals = []string{"awareness", "engagement", "lead", "conversion"}

// Limits.
const (
	MaxNameLen    = 200
	MaxTextLen    = 2000
	MaxNotesLen   = 4000
	MaxListItems  = 20
	MaxFormatQty  = 50
	MaxMembers    = 50
	MaxPlanItems  = 200
	MaxPlanText   = 1000
	DateLayout    = "2006-01-02"
	maxSlugLength = 40
)

var slug = regexp.MustCompile(`^[a-z0-9_]{1,40}$`)

// ValidDate reports whether s is YYYY-MM-DD.
func ValidDate(s string) bool {
	_, err := time.Parse(DateLayout, s)
	return err == nil
}

// Format is a requested output format and count.
type Format struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

// Brief is campaigns.brief (SPEC-04 §2.3).
type Brief struct {
	ProductIDs     []kernel.ID `json:"product_ids"`
	TargetAudience string      `json:"target_audience"`
	KeyMessage     string      `json:"key_message"`
	VerifiedOffer  string      `json:"verified_offer"`
	Channels       []string    `json:"channels"`
	Formats        []Format    `json:"formats"`
	Deadline       string      `json:"deadline"`
	Notes          string      `json:"notes"`
}

// Normalize trims text, de-duplicates lists and replaces nil slices.
func (b Brief) Normalize() Brief {
	b.TargetAudience = strings.TrimSpace(b.TargetAudience)
	b.KeyMessage = strings.TrimSpace(b.KeyMessage)
	b.VerifiedOffer = strings.TrimSpace(b.VerifiedOffer)
	b.Deadline = strings.TrimSpace(b.Deadline)
	b.Notes = strings.TrimSpace(b.Notes)
	ids := make([]kernel.ID, 0, len(b.ProductIDs))
	for _, id := range b.ProductIDs {
		if !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	b.ProductIDs = ids
	ch := make([]string, 0, len(b.Channels))
	for _, c := range b.Channels {
		c = strings.ToLower(strings.TrimSpace(c))
		if c != "" && !slices.Contains(ch, c) {
			ch = append(ch, c)
		}
	}
	b.Channels = ch
	fs := make([]Format, 0, len(b.Formats))
	for _, f := range b.Formats {
		fs = append(fs, Format{Type: strings.ToLower(strings.TrimSpace(f.Type)), Count: f.Count})
	}
	b.Formats = fs
	return b
}

func tooLong(s string, max int) bool { return utf8.RuneCountInString(s) > max }

// Validate reports every problem of a normalized brief under prefix
// ("brief"), FR-CMP-01: missing fields → 400 with one detail per field.
func (b Brief) Validate(v *kernel.ValidationErrors, prefix string) {
	f := func(s string) string { return prefix + "." + s }
	if b.TargetAudience == "" {
		v.Add(f("target_audience"), "required")
	} else if tooLong(b.TargetAudience, MaxTextLen) {
		v.Add(f("target_audience"), "too_long")
	}
	if b.KeyMessage == "" {
		v.Add(f("key_message"), "required")
	} else if tooLong(b.KeyMessage, MaxTextLen) {
		v.Add(f("key_message"), "too_long")
	}
	if tooLong(b.VerifiedOffer, MaxTextLen) {
		v.Add(f("verified_offer"), "too_long")
	}
	if tooLong(b.Notes, MaxNotesLen) {
		v.Add(f("notes"), "too_long")
	}
	if len(b.ProductIDs) > MaxListItems {
		v.Add(f("product_ids"), "too_many")
	}
	for i, id := range b.ProductIDs {
		if id == kernel.NilID {
			v.Add(fmt.Sprintf("%s.product_ids[%d]", prefix, i), "invalid")
		}
	}
	switch {
	case len(b.Channels) == 0:
		v.Add(f("channels"), "required")
	case len(b.Channels) > MaxListItems:
		v.Add(f("channels"), "too_many")
	}
	for i, c := range b.Channels {
		if !slug.MatchString(c) {
			v.Add(fmt.Sprintf("%s.channels[%d]", prefix, i), "invalid")
		}
	}
	switch {
	case len(b.Formats) == 0:
		v.Add(f("formats"), "required")
	case len(b.Formats) > MaxListItems:
		v.Add(f("formats"), "too_many")
	}
	seen := map[string]bool{}
	for i, fm := range b.Formats {
		ff := fmt.Sprintf("%s.formats[%d]", prefix, i)
		if !slug.MatchString(fm.Type) {
			v.Add(ff+".type", "invalid")
		} else if seen[fm.Type] {
			v.Add(ff+".type", "duplicate")
		}
		seen[fm.Type] = true
		if fm.Count < 1 || fm.Count > MaxFormatQty {
			v.Add(ff+".count", "out_of_range")
		}
	}
	switch {
	case b.Deadline == "":
		v.Add(f("deadline"), "required")
	case !ValidDate(b.Deadline):
		v.Add(f("deadline"), "invalid_date")
	}
}

// Campaign is a campaigns row with its members.
type Campaign struct {
	ID        kernel.ID
	BrandID   kernel.ID
	Name      string
	Goal      string
	Brief     Brief
	Status    Status
	StartDate string // YYYY-MM-DD or ""
	EndDate   string
	CreatedBy kernel.ID
	CreatedAt time.Time
	UpdatedAt time.Time
	MemberIDs []kernel.ID

	ContentPlan           *Plan
	ContentPlanApprovedBy *kernel.ID
	ContentPlanApprovedAt *time.Time
	PlanProposal          *Plan
	PlanProposedAt        *time.Time
}

// ValidateHeader checks name, goal and the date range.
func ValidateHeader(v *kernel.ValidationErrors, name, goal, start, end string) {
	switch {
	case name == "":
		v.Add("name", "required")
	case tooLong(name, MaxNameLen):
		v.Add("name", "too_long")
	}
	if goal == "" {
		v.Add("goal", "required")
	} else if !slices.Contains(Goals, goal) {
		v.Add("goal", "invalid")
	}
	if start != "" && !ValidDate(start) {
		v.Add("start_date", "invalid_date")
	}
	if end != "" && !ValidDate(end) {
		v.Add("end_date", "invalid_date")
	}
	if start != "" && end != "" && ValidDate(start) && ValidDate(end) && end < start {
		v.Add("end_date", "before_start_date")
	}
}

// IsMember reports whether userID created the campaign or is a member.
func (c *Campaign) IsMember(userID kernel.ID) bool {
	return c.CreatedBy == userID || slices.Contains(c.MemberIDs, userID)
}

// CanEdit implements "Creator chỉ sửa … thuộc campaign mình được gán;
// Lead/Admin sửa tất cả" for campaign.manage holders.
func (c *Campaign) CanEdit(a kernel.Actor) bool {
	return a.Can(kernel.PermCampaignManage) && (a.IsLeadOrAdmin() || c.IsMember(a.UserID))
}

// TransitionTo applies a status change.
func (c *Campaign) TransitionTo(to Status) error {
	if !IsStatus(string(to)) {
		return kernel.Validation(kernel.FieldIssue("status", "invalid"))
	}
	if c.Status == to {
		return nil
	}
	if !CanTransition(c.Status, to) {
		return kernel.InvalidTransition(string(c.Status), string(to))
	}
	c.Status = to
	return nil
}
