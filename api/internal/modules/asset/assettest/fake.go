// Package assettest provides an in-memory asset usecase.API.
package assettest

import (
	"context"
	"sync"
	"time"

	"github.com/be/acs/api/internal/modules/asset/usecase"
	"github.com/be/acs/api/internal/shared/kernel"
)

// API is the fake.
type API struct {
	mu       sync.Mutex
	Assets   map[kernel.ID]*usecase.AssetView
	Versions map[kernel.ID][]usecase.VersionView // by asset
}

// NewAPI returns an empty fake.
func NewAPI() *API {
	return &API{Assets: map[kernel.ID]*usecase.AssetView{}, Versions: map[kernel.ID][]usecase.VersionView{}}
}

var _ usecase.API = (*API)(nil)

func (a *API) CreateAsset(_ context.Context, in usecase.CreateAssetInput) (usecase.AssetRef, error) {
	if in.Title == "" || in.BrandID == kernel.NilID {
		return usecase.AssetRef{}, kernel.Validation(kernel.FieldIssue("title", "required"))
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now().UTC()
	id := kernel.NewID()
	a.Assets[id] = &usecase.AssetView{
		ID: id, CampaignID: in.CampaignID, BrandID: in.BrandID, Type: in.Type, Subtype: in.Subtype, Title: in.Title,
		Status: usecase.StatusDraft, SourceAssetID: in.SourceAssetID, Tags: in.Tags, CreatedBy: in.CreatedBy, CreatedAt: now, UpdatedAt: now,
	}
	return a.addVersion(id, in.Version), nil
}

func (a *API) addVersion(assetID kernel.ID, v usecase.NewVersion) usecase.AssetRef {
	no := len(a.Versions[assetID]) + 1
	vv := usecase.VersionView{
		ID: kernel.NewID(), VersionNo: no, ContentJSON: v.ContentJSON, ChangeNote: v.ChangeNote,
		GeneratedByJobID: v.GeneratedByJobID, CreatedBy: v.CreatedBy, CreatedAt: time.Now().UTC(), Files: v.Files,
	}
	a.Versions[assetID] = append(a.Versions[assetID], vv)
	as := a.Assets[assetID]
	as.CurrentVersion = &vv
	if as.Status != usecase.StatusArchived {
		as.Status = usecase.StatusDraft // any new version must be reviewed again
	}
	return usecase.AssetRef{AssetID: assetID, VersionID: vv.ID, VersionNo: no}
}

func (a *API) AddVersion(_ context.Context, assetID kernel.ID, v usecase.NewVersion) (usecase.AssetRef, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	as, ok := a.Assets[assetID]
	if !ok {
		return usecase.AssetRef{}, kernel.ErrNotFound
	}
	if as.Status == usecase.StatusArchived {
		return usecase.AssetRef{}, kernel.InvalidTransition(string(as.Status), "new_version")
	}
	return a.addVersion(assetID, v), nil
}

func (a *API) AttachFiles(_ context.Context, versionID kernel.ID, files []usecase.VersionFile) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	for assetID, vs := range a.Versions {
		for i := range vs {
			if vs[i].ID == versionID {
				vs[i].Files = append(vs[i].Files, files...)
				if cv := a.Assets[assetID].CurrentVersion; cv != nil && cv.ID == versionID {
					cv.Files = vs[i].Files
				}
				return nil
			}
		}
	}
	return kernel.ErrNotFound
}

func (a *API) Get(_ context.Context, id kernel.ID) (usecase.AssetView, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	as, ok := a.Assets[id]
	if !ok {
		return usecase.AssetView{}, kernel.ErrNotFound
	}
	return *as, nil
}

func (a *API) GetVersion(_ context.Context, id kernel.ID, no int) (usecase.VersionView, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	vs := a.Versions[id]
	if no < 1 || no > len(vs) {
		return usecase.VersionView{}, kernel.ErrNotFound
	}
	return vs[no-1], nil
}

func (a *API) CanEdit(_ context.Context, actor kernel.Actor, id kernel.ID) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	as, ok := a.Assets[id]
	if !ok {
		return kernel.ErrNotFound
	}
	if !actor.CanModifyOwned(as.CreatedBy) {
		return kernel.ErrForbidden
	}
	return nil
}

func (a *API) CountByStatus(_ context.Context, createdBy *kernel.ID) (map[usecase.Status]int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := map[usecase.Status]int{}
	for _, as := range a.Assets {
		if createdBy == nil || as.CreatedBy == *createdBy {
			out[as.Status]++
		}
	}
	return out, nil
}
