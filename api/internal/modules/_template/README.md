# Module template

Copy this directory to `internal/modules/<name>/`, rename package `template`
→ `<name>`, replace the `Note` example, then add ONE line to
`internal/app/modules.go`. See `docs/CONVENTIONS.md`.

The leading underscore keeps it out of `go build ./...`; its tests run via
`make test` (`go test ./internal/modules/_template/...`).

Postgres adapter: see `internal/modules/identity/adapter/postgres` for the
sqlc pattern (queries.sql + repository.go, entry in `sqlc.yaml`).
