// Package migrations exposes the SQL migration files to golang-migrate so the
// binary is self-contained and no file system layout has to be reproduced on
// the target host.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS

// ReferenceDataVersion is the last migration that belongs in every
// environment. Everything above it is demo data and is applied only when
// MIGRATE_INCLUDE_DEMO is on (never in PRD — see config.validate).
const ReferenceDataVersion uint = 2

// LatestVersion is the highest migration in this package.
const LatestVersion uint = 3
