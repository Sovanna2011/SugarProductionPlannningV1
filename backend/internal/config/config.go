// Package config loads typed configuration from the environment (and an
// optional .env file). Nothing outside this package reads os.Getenv.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Env      string // DEV | SIT | UAT | PRD
	HTTPAddr string
	LogLevel string

	DB       DatabaseConfig
	JWT      JWTConfig
	Security SecurityConfig
	Business BusinessConfig
	CORS     CORSConfig
}

type DatabaseConfig struct {
	Host            string
	Port            int
	User            string
	Password        string
	Name            string
	SSLMode         string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	SlowQuery       time.Duration
	AutoMigrate     bool
	IncludeDemoData bool
}

func (d DatabaseConfig) DSN() string {
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		d.Host, d.Port, d.User, d.Password, d.Name, d.SSLMode)
}

type JWTConfig struct {
	Secret     string
	Issuer     string
	AccessTTL  time.Duration
	RefreshTTL time.Duration
}

type SecurityConfig struct {
	PasswordMinLength   int
	MaxFailedLogins     int
	PermissionCacheTTL  time.Duration
	LoginRateLimit      int // attempts per window, per IP and per username
	LoginRateWindow     time.Duration
	BootstrapAdminUser  string
	BootstrapAdminPass  string
	BootstrapAdminEmail string
	SeedDemoUsers       bool
	DemoUserPassword    string
}

type BusinessConfig struct {
	// §F3 / OQ-5: the approver of a planning version must not be its creator.
	EnforceFourEyes bool
	// §F4 rule 6: how far into the future a document may be posted.
	MaxFuturePostingDays int
	// Part G safety rails for the data browser.
	BrowserRowCap    int
	BrowserExportCap int
	BrowserTimeout   time.Duration
	// §F5 traffic-light thresholds for capacity utilisation.
	CapacityAmberPct float64
	CapacityRedPct   float64
}

type CORSConfig struct {
	AllowedOrigins []string
}

// Load reads configuration, applying documented defaults. It fails fast in
// PRD when a production-unsafe default would otherwise be used.
func Load() (*Config, error) {
	_ = godotenv.Load() // .env is optional; real environments inject variables

	cfg := &Config{
		Env:      env("APP_ENV", "DEV"),
		HTTPAddr: env("HTTP_ADDR", ":8080"),
		LogLevel: env("LOG_LEVEL", "info"),
		DB: DatabaseConfig{
			Host:            env("DB_HOST", "localhost"),
			Port:            envInt("DB_PORT", 5432),
			User:            env("DB_USER", "sugar"),
			Password:        env("DB_PASSWORD", "sugar"),
			Name:            env("DB_NAME", "sugar_dev"),
			SSLMode:         env("DB_SSLMODE", "disable"),
			MaxOpenConns:    envInt("DB_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    envInt("DB_MAX_IDLE_CONNS", 5),
			ConnMaxLifetime: envDuration("DB_CONN_MAX_LIFETIME", time.Hour),
			SlowQuery:       envDuration("DB_SLOW_QUERY_THRESHOLD", 500*time.Millisecond),
			AutoMigrate:     envBool("DB_AUTO_MIGRATE", true),
			IncludeDemoData: envBool("MIGRATE_INCLUDE_DEMO", true),
		},
		JWT: JWTConfig{
			Secret:     env("JWT_SECRET", "change-me-in-every-environment"),
			Issuer:     env("JWT_ISSUER", "sugar-planning"),
			AccessTTL:  envDuration("JWT_ACCESS_TTL", 15*time.Minute),
			RefreshTTL: envDuration("JWT_REFRESH_TTL", 8*time.Hour),
		},
		Security: SecurityConfig{
			PasswordMinLength:   envInt("PASSWORD_MIN_LENGTH", 12),
			MaxFailedLogins:     envInt("MAX_FAILED_LOGINS", 5),
			PermissionCacheTTL:  envDuration("PERMISSION_CACHE_TTL", 60*time.Second),
			LoginRateLimit:      envInt("LOGIN_RATE_LIMIT", 10),
			LoginRateWindow:     envDuration("LOGIN_RATE_WINDOW", time.Minute),
			BootstrapAdminUser:  env("BOOTSTRAP_ADMIN_USER", "admin"),
			BootstrapAdminPass:  env("BOOTSTRAP_ADMIN_PASSWORD", ""),
			BootstrapAdminEmail: env("BOOTSTRAP_ADMIN_EMAIL", "admin@example.com"),
			SeedDemoUsers:       envBool("SEED_DEMO_USERS", true),
			DemoUserPassword:    env("DEMO_USER_PASSWORD", "SugarPlanning#2026"),
		},
		Business: BusinessConfig{
			EnforceFourEyes:      envBool("ENFORCE_FOUR_EYES", true),
			MaxFuturePostingDays: envInt("MAX_FUTURE_POSTING_DAYS", 1),
			BrowserRowCap:        envInt("BROWSER_ROW_CAP", 10000),
			BrowserExportCap:     envInt("BROWSER_EXPORT_CAP", 100000),
			BrowserTimeout:       envDuration("BROWSER_STATEMENT_TIMEOUT", 30*time.Second),
			CapacityAmberPct:     envFloat("CAPACITY_AMBER_PCT", 70),
			CapacityRedPct:       envFloat("CAPACITY_RED_PCT", 90),
		},
		CORS: CORSConfig{
			AllowedOrigins: envList("CORS_ALLOWED_ORIGINS", []string{"http://localhost:8081"}),
		},
	}

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) IsProduction() bool { return strings.EqualFold(c.Env, "PRD") }

func (c *Config) validate() error {
	if c.IsProduction() {
		if c.JWT.Secret == "change-me-in-every-environment" || len(c.JWT.Secret) < 32 {
			return fmt.Errorf("JWT_SECRET must be set to at least 32 characters in PRD")
		}
		if c.DB.SSLMode == "disable" {
			return fmt.Errorf("DB_SSLMODE must not be 'disable' in PRD")
		}
		if c.DB.IncludeDemoData {
			return fmt.Errorf("MIGRATE_INCLUDE_DEMO must be false in PRD")
		}
		if c.Security.SeedDemoUsers {
			return fmt.Errorf("SEED_DEMO_USERS must be false in PRD")
		}
	}
	if c.Business.CapacityRedPct <= c.Business.CapacityAmberPct {
		return fmt.Errorf("CAPACITY_RED_PCT must be greater than CAPACITY_AMBER_PCT")
	}
	return nil
}

func env(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v, err := strconv.Atoi(env(key, "")); err == nil {
		return v
	}
	return fallback
}

func envFloat(key string, fallback float64) float64 {
	if v, err := strconv.ParseFloat(env(key, ""), 64); err == nil {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	if v, err := strconv.ParseBool(env(key, "")); err == nil {
		return v
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if v, err := time.ParseDuration(env(key, "")); err == nil {
		return v
	}
	return fallback
}

func envList(key string, fallback []string) []string {
	raw := env(key, "")
	if raw == "" {
		return fallback
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
