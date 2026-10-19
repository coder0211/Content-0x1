// Package app is the composition root shared by cmd/server, cmd/worker and
// cmd/acs: it builds the platform and constructs every module in order.
package app

import (
	"github.com/be/acs/api/internal/app/registry"
	"github.com/be/acs/api/internal/modules/asset"
	"github.com/be/acs/api/internal/modules/audit"
	"github.com/be/acs/api/internal/modules/brand"
	"github.com/be/acs/api/internal/modules/budget"
	"github.com/be/acs/api/internal/modules/campaign"
	"github.com/be/acs/api/internal/modules/content"
	"github.com/be/acs/api/internal/modules/dashboard"
	"github.com/be/acs/api/internal/modules/file"
	"github.com/be/acs/api/internal/modules/identity"
	"github.com/be/acs/api/internal/modules/image"
	"github.com/be/acs/api/internal/modules/job"
	"github.com/be/acs/api/internal/modules/provider"
	"github.com/be/acs/api/internal/modules/video"
)

// Modules lists every module constructor in dependency order (a module may
// only read APIs published by modules above it; SPEC-06 §2.5).
// ADD EXACTLY ONE LINE PER NEW MODULE, at the position its dependencies allow.
var Modules = []registry.Constructor{
	audit.Register,
	identity.Register,
	file.Register,
	provider.Register,
	job.Register,
	budget.Register,
	brand.Register,
	campaign.Register,
	asset.Register,
	content.Register,
	image.Register,
	video.Register,
	dashboard.Register,
}
