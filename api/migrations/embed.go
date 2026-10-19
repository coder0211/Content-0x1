// Package migrations embeds the goose SQL migrations so the acs CLI can run
// them without the source tree. Numbering: 0001–0011 are the base schema
// (SPEC-03); later schema changes use 0012+ with the owning module's name in
// the file name, e.g. 0012_video_add_scene_seed.sql.
package migrations

import "embed"

// FS holds every *.sql migration.
//
//go:embed *.sql
var FS embed.FS
