// Package database owns the connection pool, the migration runner and the
// UnitOfWork abstraction. Per §A4 no other package may open a connection —
// NewDB is called from main.go and from test setup only.
package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/config"
)

// NewDB opens the single *gorm.DB used by the whole process.
func NewDB(cfg *config.Config, log zerolog.Logger) (*gorm.DB, error) {
	gormCfg := &gorm.Config{
		Logger:                 newGormLogger(log, cfg.DB.SlowQuery),
		SkipDefaultTransaction: true, // services own their transaction boundaries (§D3)
		NowFunc:                func() time.Time { return time.Now().UTC() },
	}

	db, err := gorm.Open(postgres.Open(cfg.DB.DSN()), gormCfg)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("access sql.DB: %w", err)
	}
	sqlDB.SetMaxOpenConns(cfg.DB.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.DB.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.DB.ConnMaxLifetime)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return db, nil
}

// --- gorm ↔ zerolog bridge ----------------------------------------------

type gormLogger struct {
	log           zerolog.Logger
	slowThreshold time.Duration
}

func newGormLogger(log zerolog.Logger, slow time.Duration) gormlogger.Interface {
	return &gormLogger{log: log.With().Str("component", "gorm").Logger(), slowThreshold: slow}
}

func (l *gormLogger) LogMode(gormlogger.LogLevel) gormlogger.Interface { return l }

func (l *gormLogger) Info(_ context.Context, msg string, data ...any) {
	l.log.Info().Msgf(msg, data...)
}

func (l *gormLogger) Warn(_ context.Context, msg string, data ...any) {
	l.log.Warn().Msgf(msg, data...)
}

func (l *gormLogger) Error(_ context.Context, msg string, data ...any) {
	l.log.Error().Msgf(msg, data...)
}

func (l *gormLogger) Trace(_ context.Context, begin time.Time,
	fc func() (string, int64), err error) {

	elapsed := time.Since(begin)
	switch {
	case err != nil && !errors.Is(err, gorm.ErrRecordNotFound):
		sql, rows := fc()
		l.log.Error().Err(err).Dur("elapsed", elapsed).Int64("rows", rows).Str("sql", sql).Msg("query failed")
	case l.slowThreshold > 0 && elapsed > l.slowThreshold:
		// §D5 requires a slow-query log above the configured threshold.
		sql, rows := fc()
		l.log.Warn().Dur("elapsed", elapsed).Int64("rows", rows).Str("sql", sql).Msg("slow query")
	}
}
