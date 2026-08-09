package database

import (
	"database/sql"
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

// RunMigrations applies the embedded migrations. When demo data is switched
// off the runner stops at the last reference-data migration, so DEV and PRD
// share exactly the same structural and customizing migrations.
//
// The migrator gets its own short-lived connection rather than the shared GORM
// pool: migrate.Migrate.Close() closes the database handle it was given, which
// would otherwise take the application's pool down with it.
func RunMigrations(cfg *config.Config, log zerolog.Logger) error {
	target := migrations.ReferenceDataVersion
	if cfg.DB.IncludeDemoData {
		target = migrations.LatestVersion
	}

	err := withMigrator(cfg, func(m *migrate.Migrate) error { return m.Migrate(target) })
	switch {
	case errors.Is(err, migrate.ErrNoChange):
		log.Info().Uint("version", target).Msg("database schema already up to date")
		return nil
	case err != nil:
		return fmt.Errorf("apply migrations: %w", err)
	}

	log.Info().Uint("version", target).Bool("demoData", cfg.DB.IncludeDemoData).
		Msg("migrations applied")
	return nil
}

// MigrateDown rolls back to the given version (0 = empty database). Used by
// integration tests and by the controlled UAT/PRD rollback job — never on
// start-up.
func MigrateDown(cfg *config.Config, target uint) error {
	err := withMigrator(cfg, func(m *migrate.Migrate) error {
		if target == 0 {
			return m.Down()
		}
		return m.Migrate(target)
	})
	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate down to %d: %w", target, err)
	}
	return nil
}

func withMigrator(cfg *config.Config, fn func(*migrate.Migrate) error) error {
	sqlDB, err := sql.Open("postgres", cfg.DB.DSN())
	if err != nil {
		return fmt.Errorf("open migration connection: %w", err)
	}
	defer sqlDB.Close()

	driver, err := migratepg.WithInstance(sqlDB, &migratepg.Config{})
	if err != nil {
		return fmt.Errorf("migration driver: %w", err)
	}
	source, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("migration source: %w", err)
	}
	defer source.Close()

	m, err := migrate.NewWithInstance("iofs", source, "postgres", driver)
	if err != nil {
		return fmt.Errorf("migrator: %w", err)
	}
	return fn(m)
}
