// Command acs is the operator CLI:
//
//	acs migrate up|down|down-to <version>|status
//	acs admin create --email <e> --name <n>   (password from ACS_ADMIN_PASSWORD or stdin)
//	acs seed demo                             (idempotent demo admin for local use)
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/be/acs/api/internal/app"
	"github.com/be/acs/api/internal/app/registry"
	"github.com/be/acs/api/internal/modules/identity"
	identityuc "github.com/be/acs/api/internal/modules/identity/usecase"
	"github.com/be/acs/api/internal/platform/config"
	applog "github.com/be/acs/api/internal/platform/log"
	"github.com/be/acs/api/internal/platform/postgres"
	"github.com/be/acs/api/internal/shared/kernel"
	"github.com/be/acs/api/migrations"
)

const usage = `usage:
  acs migrate up|down|status|down-to <version>
  acs admin create --email <email> --name <name>
  acs seed demo`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) < 2 {
		return errors.New(usage)
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx := context.Background()
	log := applog.New(cfg.LogLevel)
	switch args[0] + " " + args[1] {
	case "migrate up", "migrate down", "migrate status", "migrate down-to":
		return migrate(ctx, cfg, log, args[1:])
	case "admin create":
		fs := flag.NewFlagSet("admin create", flag.ContinueOnError)
		email := fs.String("email", "", "admin e-mail")
		name := fs.String("name", "Administrator", "display name")
		if err := fs.Parse(args[2:]); err != nil {
			return err
		}
		pw := os.Getenv("ACS_ADMIN_PASSWORD")
		if pw == "" {
			fmt.Fprint(os.Stderr, "password: ")
			line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
			pw = strings.TrimSpace(line)
		}
		return createAdmin(ctx, cfg, *email, *name, pw)
	case "seed demo":
		err := createAdmin(ctx, cfg, "admin@acs.local", "Quản trị viên", "admin12345!")
		if errors.Is(err, kernel.ErrConflict) {
			fmt.Println("demo admin already exists: admin@acs.local")
			return nil
		}
		if err == nil {
			fmt.Println("demo admin: admin@acs.local / admin12345! (local only)")
		}
		return err
	}
	return errors.New(usage)
}

func migrate(ctx context.Context, cfg config.Config, log *slog.Logger, args []string) error {
	m, err := postgres.NewMigrator(cfg.DatabaseURL, migrations.FS)
	if err != nil {
		return err
	}
	defer m.Close()
	switch args[0] {
	case "up":
		return m.Up(ctx, log)
	case "down":
		return m.Down(ctx, log)
	case "status":
		return m.Status(ctx, log)
	case "down-to":
		if len(args) < 2 {
			return errors.New("down-to needs a version")
		}
		v, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil {
			return err
		}
		return m.DownTo(ctx, v, log)
	}
	return errors.New(usage)
}

func createAdmin(ctx context.Context, cfg config.Config, email, name, pw string) error {
	if email == "" {
		return errors.New("--email is required")
	}
	a, err := app.New(ctx, cfg, app.Options{Process: registry.ProcessCLI})
	if err != nil {
		return err
	}
	defer a.Close()
	for _, m := range a.Modules {
		if im, ok := m.(*identity.Module); ok {
			_, err := im.Users.Bootstrap(ctx, identityuc.CreateUserInput{Email: email, Name: name, RoleCodes: []string{kernel.RoleAdmin}, TempPassword: pw})
			if err == nil {
				fmt.Println("created admin", email)
			}
			return err
		}
	}
	return errors.New("identity module not registered")
}
