// Package migrations exposes the SQL migration files to golang-migrate so the
// binary is self-contained and no file system layout has to be reproduced on
// the target host.
//
// There are two independent migration sources:
//
//   - the root, which holds the structural and customizing migrations that
//     every environment gets, DEV through PRD;
//   - demo/, which holds demo data and is applied only when
//     MIGRATE_INCLUDE_DEMO is on (never in PRD — see config.validate).
//
// They are separate sources with separate version sequences and separate
// tracking tables, so reference data can always be added after demo data.
// Before the split the two shared one sequence, which meant a new reference
// migration could only ever be numbered below the demo one — an ordering that
// could not survive the next change.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS

//go:embed demo/*.sql
var DemoFS embed.FS

// Latest is the highest structural / reference migration.
//
// Version 3 is deliberately absent: it was the demo master data, which moved
// to demo/0001 when the sources were split. Retiring the number rather than
// reusing it makes a database migrated before the split fail loudly instead of
// silently treating the cane schema as already applied.
const Latest uint = 4

// DemoLatest is the highest migration of the demo source.
const DemoLatest uint = 2

// DemoMigrationsTable keeps the demo source's bookkeeping out of the table the
// structural migrations use.
const DemoMigrationsTable = "schema_migrations_demo"
