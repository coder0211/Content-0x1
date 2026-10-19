// Package usecase is the public contract of the campaign module.
package usecase

import (
	"context"
	"encoding/json"

	"github.com/be/acs/api/internal/shared/kernel"
)

// Status mirrors campaigns.status.
type Status string

const (
	StatusDraft      Status = "draft"
	StatusInProgress Status = "in_progress"
	StatusReview     Status = "review"
	StatusCompleted  Status = "completed"
	StatusArchived   Status = "archived"
)

// Format is a requested output format and count.
type Format struct {
	Type  string
	Count int
}

// Brief is the shared marketing brief (campaigns.brief).
type Brief struct {
	ProductIDs     []kernel.ID
	TargetAudience string
	KeyMessage     string
	VerifiedOffer  string
	Channels       []string
	Formats        []Format
	Deadline       string // YYYY-MM-DD
	Notes          string
}

// CampaignForGeneration is what generation use cases need.
type CampaignForGeneration struct {
	ID          kernel.ID
	BrandID     kernel.ID
	Name        string
	Goal        string // awareness | engagement | lead | conversion
	Status      Status
	Brief       Brief
	ContentPlan json.RawMessage
	MemberIDs   []kernel.ID
	CreatedBy   kernel.ID
}

// API is the campaign module's public interface.
type API interface {
	// GetCampaignForGeneration checks that actor may generate in the
	// campaign (generate.run; not archived/completed → 409
	// INVALID_STATE_TRANSITION) and returns its brief. NOT_FOUND when deleted.
	GetCampaignForGeneration(ctx context.Context, actor kernel.Actor, campaignID kernel.ID) (CampaignForGeneration, error)
	// IsMember reports whether userID is the creator or a member.
	IsMember(ctx context.Context, campaignID, userID kernel.ID) (bool, error)
	// SaveContentPlan stores an AI plan result (job content_plan) as a
	// proposal for the user to approve; it does not overwrite an approved plan.
	SaveContentPlanProposal(ctx context.Context, campaignID kernel.ID, plan json.RawMessage) error
}
