// Package file wires the file module (ACS-024): private object storage,
// presigned uploads with server-side verification, signed downloads.
// Decision: files live in their own module (not asset) because brand
// assets, image references, scene sources and renders all reference
// asset_files; asset versions link to files through file.API.
package file

import (
	"github.com/go-chi/chi/v5"

	"github.com/be/acs/api/internal/app/registry"
	filehttp "github.com/be/acs/api/internal/modules/file/adapter/http"
	"github.com/be/acs/api/internal/modules/file/adapter/postgres"
	"github.com/be/acs/api/internal/modules/file/domain"
	"github.com/be/acs/api/internal/modules/file/usecase"
)

// Module is the file module.
type Module struct {
	Svc     *usecase.Service
	handler *filehttp.Handler
}

// Name implements registry.Module.
func (*Module) Name() string { return "file" }

// Routes implements registry.HTTPModule.
func (m *Module) Routes(r chi.Router) { m.handler.Routes(r) }

// Register builds the module and publishes APIs.Files.
func Register(c *registry.Container) (registry.Module, error) {
	u := c.Config.Upload
	svc := &usecase.Service{
		Repo: postgres.NewRepository(c.DB), Storage: c.Storage, Prober: c.Prober, Clock: c.Clock,
		Limits: domain.Limits{Image: u.MaxImageBytes, Video: u.MaxVideoBytes, Audio: u.MaxAudioBytes, Document: u.MaxDocumentBytes},
		URLTTL: c.Config.SignedURLTTL, TempDir: c.Config.FFmpeg.TempDir, Logger: c.Logger,
	}
	c.APIs.Files = svc
	return &Module{Svc: svc, handler: &filehttp.Handler{Svc: svc}}, nil
}
