// Package tests holds the integration tests. They boot the real dependency
// graph from internal/app against a real PostgreSQL, so what they exercise is
// the production wiring — including the middleware chain, the database
// triggers and the exclusion constraints — rather than a mock of it.
//
// The suite skips itself when no database is reachable, so `go test ./...` is
// still useful on a machine without one. Point it at an instance with:
//
//	TEST_DB_HOST=localhost TEST_DB_USER=sugar TEST_DB_PASSWORD=sugar go test ./tests/...
package tests

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/rs/zerolog"

	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/app"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/config"
)

const testPassword = "SugarPlanning#2026"

var testApp *app.App

func TestMain(m *testing.M) {
	cfg, err := testConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration tests skipped: %v\n", err)
		os.Exit(0)
	}

	log := zerolog.New(io.Discard)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	testApp, err = app.New(ctx, cfg, log)
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration tests skipped: %v\n", err)
		os.Exit(0)
	}

	code := m.Run()
	_ = testApp.Close()
	os.Exit(code)
}

// testConfig builds a configuration pointing at a disposable test database,
// creating it when it does not exist yet.
func testConfig() (*config.Config, error) {
	host := env("TEST_DB_HOST", "127.0.0.1")
	port := env("TEST_DB_PORT", "5432")
	user := env("TEST_DB_USER", "sugar")
	password := env("TEST_DB_PASSWORD", "sugar")
	name := env("TEST_DB_NAME", "sugar_test")

	admin := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=postgres sslmode=disable",
		host, port, user, password)
	db, err := sql.Open("postgres", admin)
	if err != nil {
		return nil, fmt.Errorf("no test database available: %w", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("no test database available: %w", err)
	}

	// A fresh database per run keeps the assertions independent of whatever a
	// previous run left behind.
	if _, err := db.Exec("DROP DATABASE IF EXISTS " + quoteIdent(name)); err != nil {
		return nil, err
	}
	if _, err := db.Exec("CREATE DATABASE " + quoteIdent(name)); err != nil {
		return nil, err
	}

	for key, value := range map[string]string{
		"APP_ENV": "DEV", "DB_HOST": host, "DB_PORT": port, "DB_USER": user,
		"DB_PASSWORD": password, "DB_NAME": name, "DB_SSLMODE": "disable",
		"JWT_SECRET":         "integration-test-signing-key-not-for-real-use",
		"DEMO_USER_PASSWORD": testPassword, "BOOTSTRAP_ADMIN_PASSWORD": testPassword,
		"MIGRATE_INCLUDE_DEMO": "true", "SEED_DEMO_USERS": "true",
		// A generous limit keeps the repeated logins of the suite from
		// tripping the login throttle.
		"LOGIN_RATE_LIMIT": "10000",
	} {
		if err := os.Setenv(key, value); err != nil {
			return nil, err
		}
	}

	return config.Load()
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func quoteIdent(name string) string { return `"` + name + `"` }

// --- HTTP helpers --------------------------------------------------------

type response struct {
	Status int
	Body   map[string]any
	Raw    []byte
}

// call performs a request against the in-process engine.
func call(t *testing.T, method, path, token string, body any, headers ...[2]string) response {
	t.Helper()

	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("encoding the request body: %v", err)
		}
		reader = bytes.NewReader(encoded)
	}

	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	for _, header := range headers {
		req.Header.Set(header[0], header[1])
	}

	recorder := httptest.NewRecorder()
	testApp.Engine.ServeHTTP(recorder, req)

	out := response{Status: recorder.Code, Raw: recorder.Body.Bytes()}
	if len(out.Raw) > 0 {
		_ = json.Unmarshal(out.Raw, &out.Body)
	}
	return out
}

// errorCode returns the business error code of a failed response.
func (r response) errorCode() string {
	payload, ok := r.Body["error"].(map[string]any)
	if !ok {
		return ""
	}
	code, _ := payload["code"].(string)
	return code
}

// data returns the response payload as an object.
func (r response) data(t *testing.T) map[string]any {
	t.Helper()
	payload, ok := r.Body["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected an object payload, got: %s", string(r.Raw))
	}
	return payload
}

// list returns the response payload as an array.
func (r response) list(t *testing.T) []any {
	t.Helper()
	payload, ok := r.Body["data"].([]any)
	if !ok {
		t.Fatalf("expected an array payload, got: %s", string(r.Raw))
	}
	return payload
}

func login(t *testing.T, username string) string {
	t.Helper()
	res := call(t, http.MethodPost, "/api/v1/auth/login", "",
		map[string]string{"username": username, "password": testPassword})
	if res.Status != http.StatusOK {
		t.Fatalf("login as %s failed with %d: %s", username, res.Status, string(res.Raw))
	}
	tokens, ok := res.data(t)["tokens"].(map[string]any)
	if !ok {
		t.Fatalf("login response carried no tokens: %s", string(res.Raw))
	}
	return tokens["accessToken"].(string)
}

// id extracts an int64 from a decoded JSON number.
func id(value any) int64 {
	number, ok := value.(float64)
	if !ok {
		return 0
	}
	return int64(number)
}

// findBy returns the first list entry whose field equals value.
func findBy(rows []any, field, value string) map[string]any {
	for _, row := range rows {
		entry, ok := row.(map[string]any)
		if !ok {
			continue
		}
		if entry[field] == value {
			return entry
		}
	}
	return nil
}

func today() string { return time.Now().UTC().Format("2006-01-02") }

func daysFromNow(days int) string {
	return time.Now().UTC().AddDate(0, 0, days).Format("2006-01-02")
}
