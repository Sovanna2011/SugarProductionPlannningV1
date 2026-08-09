package database

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	migratepg "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/lib/pq" // database/sql driver used only by the migrator
	"github.com/rs/zerolog"

	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/config"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/migrations"
)

// RunMigrations applies the embedded migrations. The structural and reference
// migrations run in every environment; the demo source runs only when demo
// data is switched on, so DEV and PRD share exactly the same schema.
//
// The migrator gets its own short-lived connection rather than the shared GORM
// pool: migrate.Migrate.Close() closes the database handle it was given, which
// would otherwise take the application's pool down with it.
func RunMigrations(cfg *config.Config, log zerolog.Logger) error {
	if err := apply(cfg, coreSource, migrations.Latest); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	log.Info().Uint("version", migrations.Latest).Msg("schema migrations applied")

	if !cfg.DB.IncludeDemoData {
		return nil
	}
	if err := apply(cfg, demoSource, migrations.DemoLatest); err != nil {
		return fmt.Errorf("apply demo migrations: %w", err)
	}
	log.Info().Uint("version", migrations.DemoLatest).Msg("demo data applied")
	return nil
}

// MigrateDown rolls back the structural migrations to the given version
// (0 = empty database), taking the demo source down first because its data
// references the tables the structural migrations own. Used by integration
// tests and by the controlled UAT/PRD rollback job — never on start-up.
func MigrateDown(cfg *config.Config, target uint) error {
	if err := down(cfg, demoSource, 0); err != nil {
		return err
	}
	return down(cfg, coreSource, target)
}

func apply(cfg *config.Config, src source, target uint) error {
	err := withMigrator(cfg, src, func(m *migrate.Migrate) error { return m.Migrate(target) })
	if errors.Is(err, migrate.ErrNoChange) {
		return nil
	}
	return err
}

func down(cfg *config.Config, src source, target uint) error {
	err := withMigrator(cfg, src, func(m *migrate.Migrate) error {
		if target == 0 {
			return m.Down()
		}
		return m.Migrate(target)
	})
	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate %s down to %d: %w", src.name, target, err)
	}
	return nil
}

// source names one of the two migration sets. Each has its own bookkeeping
// table, so their version sequences are independent.
type source struct {
	name            string
	dir             string
	migrationsTable string
	files           embed.FS
}

var (
	coreSource = source{name: "schema", dir: ".", files: migrations.FS}
	demoSource = source{
		name: "demo", dir: "demo",
		migrationsTable: migrations.DemoMigrationsTable,
		files:           migrations.DemoFS,
	}
)

func withMigrator(cfg *config.Config, src source, fn func(*migrate.Migrate) error) error {
	sqlDB, err := sql.Open("postgres", cfg.DB.DSN())
	if err != nil {
		return fmt.Errorf("open migration connection: %w", err)
	}
	defer sqlDB.Close()

	driver, err := migratepg.WithInstance(sqlDB, &migratepg.Config{MigrationsTable: src.migrationsTable})
	if err != nil {
		return fmt.Errorf("migration driver: %w", err)
	}

	fsSource, err := iofs.New(src.files, src.dir)
	if err != nil {
		return fmt.Errorf("migration source: %w", err)
	}
	defer fsSource.Close()

	m, err := migrate.NewWithInstance("iofs", fsSource, "postgres", driver)
	if err != nil {
		return fmt.Errorf("migrator: %w", err)
	}
	return fn(m)
}
