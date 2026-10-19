package usecase

import (
	"context"
	"encoding/json"
	"mime"
	"strings"
	"time"
	"unicode"

	"github.com/be/acs/api/internal/modules/asset/domain"
	fileuc "github.com/be/acs/api/internal/modules/file/usecase"
	"github.com/be/acs/api/internal/shared/kernel"
)

// DownloadURL is a signed, audited download link.
type DownloadURL struct {
	URL       string
	ExpiresAt time.Time
	FileID    kernel.ID
}

// ErrNotApproved is returned when a Creator/Reviewer downloads an asset that
// is not approved (FR-AST-05).
var ErrNotApproved = kernel.ErrForbidden.WithMessage("Chỉ tải được asset đã được duyệt")

// Download returns a signed URL for a file of the asset (ACS-084):
//   - asset.download is required;
//   - Creator/Reviewer: only approved assets, only current-version files;
//   - Lead/Admin: any status, any version's file;
//   - every successful call writes an asset.download audit row.
func (s *Service) Download(ctx context.Context, actor kernel.Actor, id kernel.ID, fileID *kernel.ID) (DownloadURL, error) {
	a, err := s.visible(ctx, actor, id)
	if err != nil {
		return DownloadURL{}, err
	}
	if err := actor.Require(kernel.PermAssetDownload); err != nil {
		return DownloadURL{}, err
	}
	privileged := actor.IsLeadOrAdmin()
	if !privileged && a.Status != domain.StatusApproved {
		return DownloadURL{}, ErrNotApproved
	}
	if a.CurrentVersionID == nil {
		return DownloadURL{}, kernel.ErrNotFound
	}
	cur, err := s.Repo.GetVersionByID(ctx, *a.CurrentVersionID)
	if err != nil {
		return DownloadURL{}, err
	}
	var chosen *VersionFile
	versionNo := cur.VersionNo
	if fileID == nil {
		chosen = defaultDownloadFile(cur.Files)
	} else {
		chosen = findFile(cur.Files, *fileID)
		if chosen == nil && privileged {
			vs, err := s.Repo.ListVersions(ctx, a.ID)
			if err != nil {
				return DownloadURL{}, err
			}
			for _, v := range vs {
				if f := findFile(v.Files, *fileID); f != nil {
					chosen, versionNo = f, v.VersionNo
					break
				}
			}
		}
	}
	if chosen == nil && fileID == nil && a.Type == domain.TypeContent {
		// Text content has no stored file: export the current version as a
		// UTF-8 .txt document (Flow A "ready to download").
		exp, err := s.exportContent(ctx, actor, a, cur)
		if err != nil {
			return DownloadURL{}, err
		}
		chosen = &exp
	}
	if chosen == nil {
		return DownloadURL{}, kernel.ErrNotFound.WithMessage("Asset không có tệp để tải")
	}
	f, err := s.Files.Get(ctx, chosen.FileID)
	if err != nil {
		return DownloadURL{}, err
	}
	u, err := s.Files.SignedURL(ctx, chosen.FileID, downloadName(a.Title, f.OriginalName, f.MimeType))
	if err != nil {
		return DownloadURL{}, err
	}
	err = s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		return s.Audit.Record(ctx, "asset.download", "asset", a.ID.String(), nil, map[string]any{
			"file_id": chosen.FileID, "role": chosen.Role, "version_no": versionNo, "status": a.Status,
		})
	})
	if err != nil {
		return DownloadURL{}, err
	}
	return DownloadURL{URL: u.URL, ExpiresAt: u.ExpiresAt, FileID: chosen.FileID}, nil
}

// FileStorer is implemented by the file module's API (fileuc.API.Store). It
// is optional on Service.Files so test fakes without Store keep working.
type FileStorer interface {
	Store(ctx context.Context, in fileuc.StoreInput) (fileuc.File, error)
}

// exportContent stores the text export of a content version and returns it
// as a transient (not attached) primary file.
func (s *Service) exportContent(ctx context.Context, actor kernel.Actor, a *domain.Asset, v *VersionRecord) (VersionFile, error) {
	st, ok := s.Files.(FileStorer)
	text := domain.ExportText(v.ContentJSON)
	if !ok || text == "" {
		return VersionFile{}, kernel.ErrNotFound.WithMessage("Asset không có tệp để tải")
	}
	meta, _ := json.Marshal(map[string]any{"export_of_asset_id": a.ID, "version_no": v.VersionNo})
	uid := actor.UserID
	f, err := st.Store(ctx, fileuc.StoreInput{
		// Kind/Source are the file domain's KindDocument / SourceGenerated.
		Body: strings.NewReader(text), Size: int64(len(text)), MimeType: "text/plain",
		Kind: "document", Source: "generated", CreatedBy: &uid, Metadata: meta, OriginalName: "export.txt",
	})
	if err != nil {
		return VersionFile{}, err
	}
	return VersionFile{FileID: f.ID, Role: RolePrimary}, nil
}

func findFile(fs []VersionFile, id kernel.ID) *VersionFile {
	for i := range fs {
		if fs[i].FileID == id {
			return &fs[i]
		}
	}
	return nil
}

// defaultDownloadFile is the primary file with the lowest sort order, else
// the first non-thumbnail file.
func defaultDownloadFile(fs []VersionFile) *VersionFile {
	var best *VersionFile
	for i := range fs {
		if fs[i].Role == RolePrimary && (best == nil || fs[i].SortOrder < best.SortOrder) {
			best = &fs[i]
		}
	}
	if best != nil {
		return best
	}
	for i := range fs {
		if fs[i].Role != RoleThumbnail && fs[i].Role != RolePreview {
			return &fs[i]
		}
	}
	return nil
}

var commonExt = map[string]string{
	"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp", "video/mp4": ".mp4",
	"audio/mpeg": ".mp3", "text/vtt": ".vtt", "application/x-subrip": ".srt", "text/plain": ".txt",
}

// downloadName builds an ASCII-safe attachment name from the title. The
// extension comes from the verified MIME type, never from the uploader's
// original filename (a PNG named "x.html" downloads as .png, ACS-101); the
// original extension is used only for types without a known extension.
func downloadName(title, original, mimeType string) string {
	ext := ""
	if e, ok := commonExt[mimeType]; ok {
		ext = e
	} else if exts, _ := mime.ExtensionsByType(mimeType); len(exts) > 0 {
		ext = exts[0]
	} else if i := strings.LastIndexByte(original, '.'); i > 0 && len(original)-i <= 6 && isAlnum(original[i+1:]) {
		ext = strings.ToLower(original[i:])
	}
	var b strings.Builder
	for _, r := range foldVietnamese(title) {
		if b.Len() >= 80 {
			break
		}
		switch {
		case r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteByte('-')
		}
	}
	name := strings.Trim(b.String(), "-")
	if name == "" {
		name = "asset"
	}
	return name + ext
}

func isAlnum(s string) bool {
	for _, r := range s {
		if r >= unicode.MaxASCII || (!unicode.IsLetter(r) && !unicode.IsDigit(r)) {
			return false
		}
	}
	return s != ""
}

// vnFold maps Vietnamese letters to ASCII ("Ưu đãi" → "Uu dai").
var vnFold = func() map[rune]rune {
	groups := map[rune]string{
		'a': "àáạảãâầấậẩẫăằắặẳẵ", 'e': "èéẹẻẽêềếệểễ", 'i': "ìíịỉĩ", 'o': "òóọỏõôồốộổỗơờớợởỡ",
		'u': "ùúụủũưừứựửữ", 'y': "ỳýỵỷỹ", 'd': "đ",
	}
	m := map[rune]rune{}
	for base, letters := range groups {
		for _, r := range letters {
			m[r] = base
			m[unicode.ToUpper(r)] = unicode.ToUpper(base)
		}
	}
	return m
}()

func foldVietnamese(s string) string {
	return strings.Map(func(r rune) rune {
		if f, ok := vnFold[r]; ok {
			return f
		}
		return r
	}, s)
}
