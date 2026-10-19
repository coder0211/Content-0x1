package usecase

import (
	"context"
	"mime"
	"strings"
	"time"
	"unicode"

	"github.com/be/acs/api/internal/modules/asset/domain"
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
	"audio/mpeg": ".mp3", "text/vtt": ".vtt", "application/x-subrip": ".srt",
}

// downloadName builds an ASCII-safe attachment name from the title.
func downloadName(title, original, mimeType string) string {
	ext := ""
	if i := strings.LastIndexByte(original, '.'); i > 0 && len(original)-i <= 6 {
		ext = strings.ToLower(original[i:])
	} else if e, ok := commonExt[mimeType]; ok {
		ext = e
	} else if exts, _ := mime.ExtensionsByType(mimeType); len(exts) > 0 {
		ext = exts[0]
	}
	var b strings.Builder
	for _, r := range foldVietnamese(title) {
		switch {
		case r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteByte('-')
		}
		if b.Len() >= 80 {
			break
		}
	}
	name := strings.Trim(b.String(), "-")
	if name == "" {
		name = "asset"
	}
	return name + ext
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
