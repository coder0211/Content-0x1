// Package domain holds the asset entity, its review state machine
// (FR-AST-03) and pure rules (tags, search text). Imports: stdlib + kernel.
package domain

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/be/acs/api/internal/shared/kernel"
)

// Type mirrors content_assets.asset_type.
type Type string

const (
	TypeContent Type = "content"
	TypeImage   Type = "image"
	TypeVideo   Type = "video"
)

// Valid reports whether t is a known asset type.
func (t Type) Valid() bool { return t == TypeContent || t == TypeImage || t == TypeVideo }

// Status mirrors content_assets.status.
type Status string

const (
	StatusDraft            Status = "draft"
	StatusPendingReview    Status = "pending_review"
	StatusChangesRequested Status = "changes_requested"
	StatusApproved         Status = "approved"
	StatusArchived         Status = "archived"
)

// AllStatuses lists every status.
var AllStatuses = []Status{StatusDraft, StatusPendingReview, StatusChangesRequested, StatusApproved, StatusArchived}

// Valid reports whether s is a known status.
func (s Status) Valid() bool {
	for _, x := range AllStatuses {
		if x == s {
			return true
		}
	}
	return false
}

// Event is what drives a status change.
type Event string

const (
	EventSubmit         Event = "submit"
	EventApprove        Event = "approve"
	EventRequestChanges Event = "request_changes"
	EventNewVersion     Event = "new_version"
	EventArchive        Event = "archive"
)

// transitions is the complete review state machine (FR-AST-03):
//
//	draft --submit--> pending_review --approve--> approved
//	                  pending_review --request_changes--> changes_requested
//	{draft, pending_review, changes_requested, approved} --new_version--> draft
//	any non-archived --archive--> archived
//
// A new version always returns the asset to draft: content that changed
// after (or during) review must be reviewed again.
var transitions = map[Status]map[Event]Status{
	StatusDraft: {
		EventSubmit: StatusPendingReview, EventNewVersion: StatusDraft, EventArchive: StatusArchived,
	},
	StatusPendingReview: {
		EventApprove: StatusApproved, EventRequestChanges: StatusChangesRequested,
		EventNewVersion: StatusDraft, EventArchive: StatusArchived,
	},
	StatusChangesRequested: {EventNewVersion: StatusDraft, EventArchive: StatusArchived},
	StatusApproved:         {EventNewVersion: StatusDraft, EventArchive: StatusArchived},
	StatusArchived:         {},
}

// Next returns the status reached from `from` on event e, or
// INVALID_STATE_TRANSITION.
func Next(from Status, e Event) (Status, error) {
	if to, ok := transitions[from][e]; ok {
		return to, nil
	}
	return "", kernel.InvalidTransition(string(from), string(e))
}

// SubtypePoster is the image subtype that requires a typography check.
const SubtypePoster = "poster"

// Asset is a content asset (content/image/video) with review status.
type Asset struct {
	ID               kernel.ID
	CampaignID       *kernel.ID
	BrandID          kernel.ID
	Type             Type
	Subtype          string
	Title            string
	Status           Status
	CurrentVersionID *kernel.ID
	CurrentVersionNo int
	SourceAssetID    *kernel.ID
	Tags             []string
	UsageRights      *string
	CreatedBy        kernel.ID
	CurrentVersionBy kernel.ID // author of the current version
	SubmittedAt      *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        *time.Time
}

func (a *Asset) apply(e Event) error {
	to, err := Next(a.Status, e)
	if err != nil {
		return err
	}
	a.Status = to
	return nil
}

// Checklist is the reviewer checklist sent on submit (approvals.checklist).
type Checklist struct {
	TypographyChecked bool `json:"typography_checked"`
}

// RequiresTypographyCheck reports whether submit needs the poster checklist.
func (a *Asset) RequiresTypographyCheck() bool { return a.Subtype == SubtypePoster }

// Submit moves draft → pending_review. Posters require the typography check.
func (a *Asset) Submit(c Checklist, now time.Time) error {
	if a.RequiresTypographyCheck() && !c.TypographyChecked {
		return kernel.Validation(kernel.FieldIssue("checklist.typography_checked", "required")).
			WithMessage("Poster có chữ cần xác nhận đã kiểm tra typography trước khi gửi duyệt")
	}
	if err := a.apply(EventSubmit); err != nil {
		return err
	}
	a.SubmittedAt = &now
	return nil
}

// ErrSelfApproval is returned when a creator approves their own asset.
var ErrSelfApproval = kernel.NewError(kernel.CodeSelfApproval, "Bạn không thể tự duyệt asset của chính mình")

// Approve moves pending_review → approved. The asset's creator (or the
// author of the version under review) may not approve it unless Admin.
func (a *Asset) Approve(reviewer kernel.ID, isAdmin bool) error {
	if a.Status != StatusPendingReview {
		return kernel.InvalidTransition(string(a.Status), string(EventApprove))
	}
	if !isAdmin && (reviewer == a.CreatedBy || reviewer == a.CurrentVersionBy) {
		return ErrSelfApproval
	}
	return a.apply(EventApprove)
}

// RequestChanges moves pending_review → changes_requested; the comment is
// mandatory (FR-AST-03).
func (a *Asset) RequestChanges(comment string) error {
	if strings.TrimSpace(comment) == "" {
		return kernel.Validation(kernel.FieldIssue("comment", "required")).
			WithMessage("Vui lòng nhập nhận xét khi yêu cầu chỉnh sửa")
	}
	return a.apply(EventRequestChanges)
}

// OnNewVersion returns the asset to draft (archived assets are frozen).
func (a *Asset) OnNewVersion() error { return a.apply(EventNewVersion) }

// Limits for editable fields.
const (
	MaxTitleLen       = 200
	MaxTags           = 20
	MaxTagLen         = 50
	MaxUsageRightsLen = 2000
	MaxCommentLen     = 2000
	MaxChangeNoteLen  = 500
)

// NormalizeTitle trims and validates a title.
func NormalizeTitle(title string) (string, error) {
	t := strings.TrimSpace(title)
	if t == "" {
		return "", kernel.Validation(kernel.FieldIssue("title", "required"))
	}
	if utf8.RuneCountInString(t) > MaxTitleLen {
		return "", kernel.Validation(kernel.FieldIssue("title", "too_long"))
	}
	return t, nil
}

// NormalizeTags trims, lower-cases, de-duplicates and validates tags,
// keeping their first-seen order.
func NormalizeTags(tags []string) ([]string, error) {
	out := make([]string, 0, len(tags))
	seen := map[string]bool{}
	for _, raw := range tags {
		t := strings.ToLower(strings.Join(strings.Fields(raw), " "))
		if t == "" || seen[t] {
			continue
		}
		if utf8.RuneCountInString(t) > MaxTagLen {
			return nil, kernel.Validation(kernel.FieldIssue("tags", "too_long"))
		}
		seen[t] = true
		out = append(out, t)
	}
	if len(out) > MaxTags {
		return nil, kernel.Validation(kernel.FieldIssue("tags", "too_many"))
	}
	return out, nil
}

// NormalizeUsageRights trims the note; empty means "none" (nil).
func NormalizeUsageRights(s *string) (*string, error) {
	if s == nil {
		return nil, nil
	}
	t := strings.TrimSpace(*s)
	if t == "" {
		return nil, nil
	}
	if utf8.RuneCountInString(t) > MaxUsageRightsLen {
		return nil, kernel.Validation(kernel.FieldIssue("usage_rights", "too_long"))
	}
	return &t, nil
}
