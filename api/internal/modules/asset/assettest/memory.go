package assettest

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/be/acs/api/internal/modules/asset/domain"
	"github.com/be/acs/api/internal/modules/asset/usecase"
	fileuc "github.com/be/acs/api/internal/modules/file/usecase"
	identityuc "github.com/be/acs/api/internal/modules/identity/usecase"
	"github.com/be/acs/api/internal/shared/kernel"
)

// Repo is an in-memory usecase.Repository for the asset module's own
// use-case and HTTP tests. Full-text search is approximated by substring
// matching of the prefix terms against title + search body.
type Repo struct {
	mu        sync.Mutex
	assets    map[kernel.ID]*domain.Asset
	versions  map[kernel.ID][]*usecase.VersionRecord
	approvals map[kernel.ID][]usecase.Approval
	body      map[kernel.ID]string
	Prov      map[kernel.ID]usecase.Provenance
}

// NewRepo returns an empty repository.
func NewRepo() *Repo {
	return &Repo{assets: map[kernel.ID]*domain.Asset{}, versions: map[kernel.ID][]*usecase.VersionRecord{},
		approvals: map[kernel.ID][]usecase.Approval{}, body: map[kernel.ID]string{}, Prov: map[kernel.ID]usecase.Provenance{}}
}

var _ usecase.Repository = (*Repo)(nil)

func cloneAsset(a *domain.Asset) *domain.Asset {
	c := *a
	c.Tags = slices.Clone(a.Tags)
	return &c
}

func (r *Repo) CreateAsset(_ context.Context, a *domain.Asset) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.assets[a.ID] = cloneAsset(a)
	return nil
}

func (r *Repo) NextVersionNo(_ context.Context, id kernel.ID) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.versions[id]) + 1, nil
}

func (r *Repo) InsertVersion(_ context.Context, v *usecase.VersionRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := *v
	c.Files = slices.Clone(v.Files)
	r.versions[v.AssetID] = append(r.versions[v.AssetID], &c)
	return nil
}

func (r *Repo) get(id kernel.ID) (*domain.Asset, error) {
	a, ok := r.assets[id]
	if !ok || a.DeletedAt != nil {
		return nil, kernel.ErrNotFound
	}
	return cloneAsset(a), nil
}

func (r *Repo) Get(_ context.Context, id kernel.ID) (*domain.Asset, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.get(id)
}

func (r *Repo) GetForUpdate(ctx context.Context, id kernel.ID) (*domain.Asset, error) {
	return r.Get(ctx, id)
}

func (r *Repo) Save(_ context.Context, a *domain.Asset) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.assets[a.ID]; !ok {
		return kernel.ErrNotFound
	}
	r.assets[a.ID] = cloneAsset(a)
	return nil
}

func (r *Repo) SetSearch(_ context.Context, id kernel.ID, body string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.body[id] = body
	return nil
}

func (r *Repo) RefreshSearchTitle(context.Context, kernel.ID) error { return nil }

// SearchBody exposes the stored search body (tests).
func (r *Repo) SearchBody(id kernel.ID) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.body[id]
}

func (r *Repo) GetVersion(_ context.Context, id kernel.ID, no int) (*usecase.VersionRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	vs := r.versions[id]
	if no < 1 || no > len(vs) {
		return nil, kernel.ErrNotFound
	}
	c := *vs[no-1]
	return &c, nil
}

func (r *Repo) GetVersionByID(_ context.Context, vid kernel.ID) (*usecase.VersionRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, vs := range r.versions {
		for _, v := range vs {
			if v.ID == vid {
				c := *v
				return &c, nil
			}
		}
	}
	return nil, kernel.ErrNotFound
}

func (r *Repo) ListVersions(_ context.Context, id kernel.ID) ([]usecase.VersionRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]usecase.VersionRecord, 0, len(r.versions[id]))
	for i := len(r.versions[id]) - 1; i >= 0; i-- {
		out = append(out, *r.versions[id][i])
	}
	return out, nil
}

func (r *Repo) AttachFiles(_ context.Context, vid kernel.ID, files []usecase.VersionFile) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, vs := range r.versions {
		for _, v := range vs {
			if v.ID == vid {
				v.Files = append(v.Files, files...)
				return nil
			}
		}
	}
	return kernel.ErrNotFound
}

func (r *Repo) VersionFiles(_ context.Context, ids []kernel.ID) (map[kernel.ID][]usecase.VersionFile, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[kernel.ID][]usecase.VersionFile{}
	for _, vs := range r.versions {
		for _, v := range vs {
			if slices.Contains(ids, v.ID) {
				out[v.ID] = slices.Clone(v.Files)
			}
		}
	}
	return out, nil
}

func (r *Repo) AddApproval(_ context.Context, a usecase.Approval) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.approvals[a.AssetID] = append([]usecase.Approval{a}, r.approvals[a.AssetID]...)
	return nil
}

func (r *Repo) ListApprovals(_ context.Context, id kernel.ID) ([]usecase.Approval, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.approvals[id]), nil
}

func (r *Repo) match(a *domain.Asset, f usecase.SearchFilter) bool {
	switch {
	case a.DeletedAt != nil,
		f.Type != nil && a.Type != *f.Type,
		len(f.Statuses) > 0 && !slices.Contains(f.Statuses, a.Status),
		f.CampaignID != nil && (a.CampaignID == nil || *a.CampaignID != *f.CampaignID),
		f.BrandID != nil && a.BrandID != *f.BrandID,
		f.CreatedBy != nil && a.CreatedBy != *f.CreatedBy,
		f.Tag != "" && !slices.Contains(a.Tags, f.Tag),
		f.From != nil && a.CreatedAt.Before(*f.From),
		f.To != nil && !a.CreatedAt.Before(*f.To):
		return false
	}
	if f.Query != "" {
		hay := strings.ToLower(a.Title + " " + r.body[a.ID] + " " + strings.Join(a.Tags, " "))
		for _, t := range strings.Split(f.Query, " & ") {
			if !strings.Contains(hay, strings.TrimSuffix(t, ":*")) {
				return false
			}
		}
	}
	return true
}

func (r *Repo) list(f usecase.SearchFilter, cmp func(a, b *domain.Asset) int, after func(*domain.Asset) bool, limit int) []domain.Asset {
	r.mu.Lock()
	defer r.mu.Unlock()
	var all []*domain.Asset
	for _, a := range r.assets {
		if r.match(a, f) && after(a) {
			all = append(all, a)
		}
	}
	slices.SortFunc(all, cmp)
	out := []domain.Asset{}
	for _, a := range all {
		if len(out) == limit {
			break
		}
		out = append(out, *cloneAsset(a))
	}
	return out
}

func cmpID(a, b kernel.ID) int { return bytes.Compare(a[:], b[:]) }

func (r *Repo) Search(_ context.Context, f usecase.SearchFilter, cur *kernel.TimeIDCursor, limit int) ([]domain.Asset, error) {
	return r.list(f, func(a, b *domain.Asset) int {
		if c := b.CreatedAt.Compare(a.CreatedAt); c != 0 {
			return c
		}
		return cmpID(b.ID, a.ID)
	}, func(a *domain.Asset) bool {
		return cur == nil || a.CreatedAt.Before(cur.At) || (a.CreatedAt.Equal(cur.At) && cmpID(a.ID, cur.ID) < 0)
	}, limit), nil
}

func submitted(a *domain.Asset) time.Time {
	if a.SubmittedAt != nil {
		return *a.SubmittedAt
	}
	return a.UpdatedAt
}

func (r *Repo) Queue(_ context.Context, f usecase.SearchFilter, cur *kernel.TimeIDCursor, limit int) ([]domain.Asset, error) {
	return r.list(f, func(a, b *domain.Asset) int {
		if c := submitted(a).Compare(submitted(b)); c != 0 {
			return c
		}
		return cmpID(a.ID, b.ID)
	}, func(a *domain.Asset) bool {
		return cur == nil || submitted(a).After(cur.At) || (submitted(a).Equal(cur.At) && cmpID(a.ID, cur.ID) > 0)
	}, limit), nil
}

func (r *Repo) Provenance(_ context.Context, ids []kernel.ID) (map[kernel.ID]usecase.Provenance, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[kernel.ID]usecase.Provenance{}
	for _, id := range ids {
		if p, ok := r.Prov[id]; ok {
			out[id] = p
		}
	}
	return out, nil
}

// Files is a minimal usecase.FileReader fake: Put files, SignedURL returns
// "https://files.test/<id>?name=<downloadName>".
type Files struct {
	mu    sync.Mutex
	Items map[kernel.ID]fileuc.File
}

// NewFiles returns an empty file fake.
func NewFiles() *Files { return &Files{Items: map[kernel.ID]fileuc.File{}} }

var _ usecase.FileReader = (*Files)(nil)

func setKind[K ~string](k *K, s string) { *k = K(s) }

// Put registers a ready file of the given MIME type and returns its id.
func (f *Files) Put(mimeType, kind string) kernel.ID {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := kernel.NewID()
	fl := fileuc.File{ID: id, MimeType: mimeType, OriginalName: "out" + map[string]string{"image/png": ".png", "video/mp4": ".mp4"}[mimeType]}
	setKind(&fl.Kind, kind)
	f.Items[id] = fl
	return id
}

func (f *Files) Get(_ context.Context, id kernel.ID) (fileuc.File, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fl, ok := f.Items[id]
	if !ok {
		return fileuc.File{}, kernel.ErrNotFound
	}
	return fl, nil
}

func (f *Files) GetMany(ctx context.Context, ids []kernel.ID) ([]fileuc.File, error) {
	var out []fileuc.File
	for _, id := range ids {
		if fl, err := f.Get(ctx, id); err == nil {
			out = append(out, fl)
		}
	}
	return out, nil
}

func (f *Files) SignedURL(ctx context.Context, id kernel.ID, name string) (fileuc.SignedURL, error) {
	if _, err := f.Get(ctx, id); err != nil {
		return fileuc.SignedURL{}, err
	}
	return fileuc.SignedURL{URL: "https://files.test/" + id.String() + "?name=" + name, ExpiresAt: time.Date(2026, 10, 19, 9, 15, 0, 0, time.UTC)}, nil
}

// Identity is a minimal identityuc.API fake.
type Identity struct{ Users []identityuc.UserSummary }

var _ identityuc.API = (*Identity)(nil)

func (i *Identity) UsersWithPermission(_ context.Context, perm string) ([]identityuc.UserSummary, error) {
	var out []identityuc.UserSummary
	for _, u := range i.Users {
		for _, r := range u.Roles {
			if slices.Contains(kernel.RolePermissions[r], perm) {
				out = append(out, u)
				break
			}
		}
	}
	return out, nil
}

func (i *Identity) GetUsers(_ context.Context, ids []kernel.ID) ([]identityuc.UserSummary, error) {
	var out []identityuc.UserSummary
	for _, u := range i.Users {
		if slices.Contains(ids, u.ID) {
			out = append(out, u)
		}
	}
	return out, nil
}

func (r *Repo) CountByStatus(_ context.Context, createdBy *kernel.ID) (map[domain.Status]int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[domain.Status]int{}
	for _, a := range r.assets {
		if a.DeletedAt == nil && (createdBy == nil || a.CreatedBy == *createdBy) {
			out[a.Status]++
		}
	}
	return out, nil
}
