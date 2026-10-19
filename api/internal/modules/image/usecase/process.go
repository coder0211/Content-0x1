package usecase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"

	assetuc "github.com/be/acs/api/internal/modules/asset/usecase"
	budgetuc "github.com/be/acs/api/internal/modules/budget/usecase"
	fileuc "github.com/be/acs/api/internal/modules/file/usecase"
	"github.com/be/acs/api/internal/modules/image/domain"
	jobuc "github.com/be/acs/api/internal/modules/job/usecase"
	"github.com/be/acs/api/internal/ports"
	"github.com/be/acs/api/internal/shared/kernel"
)

// maxInputBytes bounds an input image loaded into memory for a provider.
const maxInputBytes = 25 << 20

var errCancelled = kernel.NewError("JOB_CANCELLED", "Tác vụ đã bị hủy")

// Process executes the image_generate step: provider call → usage → files
// (+ thumbnails) → asset version. Inputs come only from internal storage
// (Files.Open); provider outputs are bytes the adapter already downloaded
// (no user URLs are fetched — SSRF rule).
func (s *Service) Process(ctx context.Context, sc jobuc.StepContext) (jobuc.StepResult, error) {
	var in StepInput
	if err := sc.DecodeInput(&in); err != nil {
		return jobuc.StepResult{}, jobuc.Permanent(fmt.Errorf("decode image step input: %w", err))
	}
	job := sc.Job()
	batch, err := s.Repo.Get(ctx, in.BatchID)
	if err != nil {
		return jobuc.StepResult{}, err
	}
	if batch.VersionNo != nil && batch.AssetID != nil {
		// A previous attempt committed the asset; do not call the provider again.
		out := StepOutput{AssetID: *batch.AssetID, VersionNo: *batch.VersionNo, FileIDs: []kernel.ID{}}
		return jobuc.StepResult{Output: out, JobResult: jobResult(out)}, nil
	}
	if cancelled, err := sc.CancelRequested(ctx); err == nil && cancelled {
		return jobuc.StepResult{}, jobuc.Permanent(errCancelled)
	}
	gen, err := s.AI.Image(in.ProviderCode)
	if err != nil {
		return jobuc.StepResult{}, jobuc.Permanent(err)
	}

	_ = sc.Progress(ctx, 10, "Đang chuẩn bị ảnh đầu vào")
	refs := make([]ports.MediaInput, 0, len(in.ReferenceFileIDs))
	for _, id := range in.ReferenceFileIDs {
		m, err := s.loadMedia(ctx, id)
		if err != nil {
			return jobuc.StepResult{}, err
		}
		refs = append(refs, m)
	}
	var source *ports.MediaInput
	if in.SourceFileID != nil {
		m, err := s.loadMedia(ctx, *in.SourceFileID)
		if err != nil {
			return jobuc.StepResult{}, err
		}
		source = &m
	}

	_ = sc.Progress(ctx, 25, "Đang sinh ảnh")
	var resp ports.ImageResponse
	switch in.Operation {
	case domain.OpEdit:
		if source == nil {
			return jobuc.StepResult{}, jobuc.Permanent(errors.New("edit without source file"))
		}
		req := ports.ImageEditRequest{Model: in.ModelCode, Prompt: in.Prompt, Image: *source, AspectRatio: in.AspectRatio, Quality: in.Quality, Count: 1}
		if in.MaskFileID != nil {
			m, err := s.loadMedia(ctx, *in.MaskFileID)
			if err != nil {
				return jobuc.StepResult{}, err
			}
			req.Mask = &m
		}
		resp, err = gen.Edit(ctx, req)
	case domain.OpVariation:
		if source == nil {
			return jobuc.StepResult{}, jobuc.Permanent(errors.New("variation without source file"))
		}
		resp, err = gen.Generate(ctx, ports.ImageRequest{Model: in.ModelCode, Prompt: in.Prompt, AspectRatio: in.AspectRatio,
			Quality: in.Quality, Count: in.Count, ReferenceImages: []ports.MediaInput{*source}})
	default:
		resp, err = gen.Generate(ctx, ports.ImageRequest{Model: in.ModelCode, Prompt: in.Prompt, AspectRatio: in.AspectRatio,
			Quality: in.Quality, Count: in.Count, ReferenceImages: refs})
	}
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			// The provider may have charged before the timeout (cost_source=estimated).
			s.recordEstimatedUsage(context.WithoutCancel(ctx), sc, in)
		}
		return jobuc.StepResult{}, err
	}
	if len(resp.Images) == 0 {
		return jobuc.StepResult{}, &ports.ProviderError{Kind: ports.ErrKindUnknown, Provider: in.ProviderCode, Code: "empty_output", Message: "provider returned no image"}
	}
	stepID := sc.StepID()
	if _, err := s.Budget.RecordUsage(ctx, budgetuc.UsageInput{
		JobID: job.ID, StepID: &stepID, CampaignID: in.CampaignID, UserID: job.CreatedBy,
		ProviderCode: in.ProviderCode, ModelCode: in.ModelCode, JobType: string(job.Type), Usage: resp.Usage,
	}); err != nil {
		return jobuc.StepResult{}, err
	}

	_ = sc.Progress(ctx, 70, "Đang lưu ảnh")
	stored, err := s.storeOutputs(ctx, job, in, resp.Images)
	if err != nil {
		return jobuc.StepResult{}, err
	}

	_ = sc.Progress(ctx, 90, "Đang tạo phiên bản asset")
	out, err := s.saveVersion(ctx, job, in, stored)
	if err != nil {
		return jobuc.StepResult{}, err
	}
	return jobuc.StepResult{Output: out, JobResult: jobResult(out)}, nil
}

func jobResult(o StepOutput) map[string]any {
	return map[string]any{"asset_ids": []kernel.ID{o.AssetID}, "asset_id": o.AssetID, "version_no": o.VersionNo, "file_ids": o.FileIDs}
}

func (s *Service) loadMedia(ctx context.Context, id kernel.ID) (ports.MediaInput, error) {
	rc, f, err := s.Files.Open(ctx, id)
	if err != nil {
		if errors.Is(err, kernel.ErrNotFound) {
			return ports.MediaInput{}, jobuc.Permanent(err)
		}
		return ports.MediaInput{}, err
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, maxInputBytes+1))
	if err != nil {
		return ports.MediaInput{}, err
	}
	if len(data) > maxInputBytes {
		return ports.MediaInput{}, jobuc.Permanent(kernel.ErrFileTooLarge)
	}
	return ports.MediaInput{MimeType: f.MimeType, Data: data}, nil
}

func (s *Service) recordEstimatedUsage(ctx context.Context, sc jobuc.StepContext, in StepInput) {
	job := sc.Job()
	stepID := sc.StepID()
	count := in.Count
	if in.Operation == domain.OpEdit {
		count = 1
	}
	u := ports.Usage{Images: count, Units: map[string]float64{domain.FallbackPricingUnit(in.Quality): float64(count)}}
	if _, err := s.Budget.RecordUsage(ctx, budgetuc.UsageInput{
		JobID: job.ID, StepID: &stepID, CampaignID: in.CampaignID, UserID: job.CreatedBy, ProviderCode: in.ProviderCode,
		ModelCode: in.ModelCode, JobType: string(job.Type), Usage: u, Estimated: true,
	}); err != nil {
		s.log().WarnContext(ctx, "record estimated image usage", slog.String("job_id", job.ID.String()), slog.Any("err", err))
	}
}

type storedImage struct {
	file  fileuc.File
	thumb *fileuc.File
}

func (s *Service) storeOutputs(ctx context.Context, job jobuc.JobInfo, in StepInput, imgs []ports.GeneratedImage) ([]storedImage, error) {
	jobID, batchID := job.ID, in.BatchID
	meta, _ := json.Marshal(fileMeta{CampaignID: in.CampaignID, JobID: &jobID, BatchID: &batchID, Operation: string(in.Operation)})
	createdBy := job.CreatedBy
	out := make([]storedImage, 0, len(imgs))
	for i, img := range imgs {
		mime := img.MimeType
		if mime == "" {
			mime = "image/png"
		}
		f, err := s.Files.Store(ctx, fileuc.StoreInput{
			Body: bytes.NewReader(img.Data), Size: int64(len(img.Data)), MimeType: mime, Kind: fileKindImage,
			Source: "generated", ParentFileID: in.SourceFileID, CreatedBy: &createdBy, Width: img.Width, Height: img.Height,
			Metadata: meta, OriginalName: fmt.Sprintf("image-%d%s", i+1, extFor(mime)),
		})
		if err != nil {
			return nil, err
		}
		si := storedImage{file: f}
		if data, w, h, err := thumbnail(bytes.NewReader(img.Data)); err != nil {
			s.log().WarnContext(ctx, "image thumbnail skipped", slog.String("file_id", f.ID.String()), slog.Any("err", err))
		} else {
			fid := f.ID
			tmeta, _ := json.Marshal(fileMeta{CampaignID: in.CampaignID, JobID: &jobID, BatchID: &batchID, ThumbnailOf: &fid})
			t, err := s.Files.Store(ctx, fileuc.StoreInput{
				Body: bytes.NewReader(data), Size: int64(len(data)), MimeType: "image/jpeg", Kind: fileKindImage,
				Source: "generated", ParentFileID: &fid, CreatedBy: &createdBy, Width: w, Height: h, Metadata: tmeta,
				OriginalName: fmt.Sprintf("thumb-%d.jpg", i+1),
			})
			if err != nil {
				return nil, err
			}
			si.thumb = &t
		}
		out = append(out, si)
	}
	return out, nil
}

func extFor(mime string) string {
	switch mime {
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	}
	return ".png"
}

// saveVersion creates the asset (generation) or appends a version (edit,
// variation) and links the batch, in one transaction.
func (s *Service) saveVersion(ctx context.Context, job jobuc.JobInfo, in StepInput, imgs []storedImage) (StepOutput, error) {
	var files []assetuc.VersionFile
	content := domain.Content{
		Prompt: in.Prompt, Preset: in.Preset, AspectRatio: in.AspectRatio, Quality: in.Quality, Count: len(imgs),
		Operation: in.Operation, ProviderCode: in.ProviderCode, ModelCode: in.ModelCode,
		FileIDs: []kernel.ID{}, ThumbnailFileIDs: []kernel.ID{}, ReferenceFileIDs: in.ReferenceFileIDs,
		SourceFileID: in.SourceFileID, RequiresTypographyCheck: domain.RequiresTypographyCheck(in.Preset),
	}
	if content.ReferenceFileIDs == nil {
		content.ReferenceFileIDs = []kernel.ID{}
	}
	order := 0
	if in.Operation == domain.OpVariation && in.AssetID != nil {
		// Keep the current images; append the variants after them.
		prev, err := s.Assets.Get(ctx, *in.AssetID)
		if err != nil {
			return StepOutput{}, err
		}
		if prev.CurrentVersion != nil {
			var pc domain.Content
			_ = json.Unmarshal(prev.CurrentVersion.ContentJSON, &pc)
			for _, f := range prev.CurrentVersion.Files {
				if isImageRole(f.Role) || f.Role == assetuc.RoleThumbnail {
					files = append(files, f)
					order = max(order, f.SortOrder+1)
				}
				if isImageRole(f.Role) {
					content.FileIDs = append(content.FileIDs, f.FileID)
				}
			}
			content.ThumbnailFileIDs = append(content.ThumbnailFileIDs, pc.ThumbnailFileIDs...)
		}
	}
	role := assetuc.RolePrimary
	if in.Operation == domain.OpVariation || len(imgs) > 1 {
		role = assetuc.RoleVariant // count=4 → 4 variant files (ACS-051 AC)
	}
	newIDs := make([]kernel.ID, 0, len(imgs))
	for _, im := range imgs {
		files = append(files, assetuc.VersionFile{FileID: im.file.ID, Role: role, SortOrder: order})
		content.FileIDs = append(content.FileIDs, im.file.ID)
		newIDs = append(newIDs, im.file.ID)
		if im.thumb != nil {
			files = append(files, assetuc.VersionFile{FileID: im.thumb.ID, Role: assetuc.RoleThumbnail, SortOrder: order})
			content.ThumbnailFileIDs = append(content.ThumbnailFileIDs, im.thumb.ID)
		}
		order++
	}
	cj, err := json.Marshal(content)
	if err != nil {
		return StepOutput{}, jobuc.Permanent(err)
	}
	jobID := job.ID
	note := map[domain.Operation]string{
		domain.OpGenerate: "Sinh ảnh", domain.OpEdit: "Chỉnh sửa bằng prompt", domain.OpVariation: "Tạo biến thể",
	}[in.Operation]
	v := assetuc.NewVersion{ContentJSON: cj, ChangeNote: note, GeneratedByJobID: &jobID, CreatedBy: job.CreatedBy, Files: files, SearchText: in.Prompt}
	var out StepOutput
	err = s.Tx.WithinTx(ctx, func(ctx context.Context) error {
		var ref assetuc.AssetRef
		var err error
		if in.Operation == domain.OpGenerate || in.AssetID == nil {
			ref, err = s.Assets.CreateAsset(ctx, assetuc.CreateAssetInput{
				CampaignID: in.CampaignID, BrandID: in.BrandID, Type: assetuc.TypeImage, Subtype: string(in.Preset),
				Title: domain.Title(in.Prompt, in.Preset), Tags: []string{string(in.Preset)}, CreatedBy: job.CreatedBy, Version: v,
			})
		} else {
			ref, err = s.Assets.AddVersion(ctx, *in.AssetID, v)
		}
		if err != nil {
			return err
		}
		out = StepOutput{AssetID: ref.AssetID, VersionNo: ref.VersionNo, FileIDs: newIDs}
		return s.Repo.SetResult(ctx, in.BatchID, ref.AssetID, ref.VersionNo)
	})
	if err != nil {
		return StepOutput{}, err
	}
	return out, nil
}
