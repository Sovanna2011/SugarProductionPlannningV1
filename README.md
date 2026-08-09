# Sugar Production Planning

Multi-company production planning versus actual for a sugar mill and refinery:
plan where the cane comes from and what each production line should make on
each day, record what actually arrived and what was actually made, keep the
stock ledger that follows from it, and report the variance — across several
legally separate companies from one login.

Built to [`docs/TECHNICAL_SPECIFICATION.md`](docs/TECHNICAL_SPECIFICATION.md).
The material flow it models is in
[`docs/inventory-material-flow.mermaid`](docs/inventory-material-flow.mermaid).

---

## Running it

```bash
docker compose up --build      # → http://localhost:8081
```

That brings up PostgreSQL, the API and the UI. The database is migrated and
seeded on first start.

Development accounts (password `SugarPlanning#2026`):

| User | What it can do |
|---|---|
| `admin` | everything, in all three demo companies |
| `planner` | planner in company 1000, **display only** in 2000; also maintains growers and fields |
| `approver` | approves and locks versions in 1000 and 2000 |
| `operator` | records and posts actual production and cane deliveries, maintains stock |
| `viewer` | read-only |

`planner` is the interesting one: the same login, a different role in each
company.

### Without Docker

```bash
createdb sugar_dev
cp backend/.env.example backend/.env      # then set BOOTSTRAP_ADMIN_PASSWORD
make run                                  # API on :8080
make web                                  # UI on :8081, /api proxied to :8080
```

`make help` lists the rest.

### A prototype you can just open

[`docs/prototype/index.html`](docs/prototype/index.html) is a single
self-contained page that mirrors the screens and ports the rules — the
variance `n/a`, the null capacity, the status machine, four eyes, the silo
routing, the capacity and negative-stock refusals, purchased versus own-estate
cane. It needs no database and no network, which makes it the quickest way to
put the system in front of someone. It is not connected to the service; where
the two disagree, the service is right.

### Loading a dataset to test against

Master data alone leaves every screen empty. To put a working campaign in
front of a tester:

```bash
scripts/demo-scenario.sh                 # against http://localhost:8080
```

It drives the API as the demo users, so everything it creates is data the
business rules actually accepted. It leaves behind:

- an **approved** budget version and a **draft** forecast copied from it,
  seven days of plan across the mill and refinery lines
- a **harvest plan** in the same version: seven days of cane across two own
  estates and four purchased suppliers
- three days of cane weighed in and issued to the mill, so the yard fills and
  drains the way it does in a campaign
- three days of milling posted — raw sugar, molasses and bagasse
- the refinery route both ways: refined sugar conditioned in the silo and then
  packed, white sugar straight to its own warehouse
- bagasse burnt for electricity, which is produced but never becomes stock
- a molasses transfer between two tanks
- **one draft production document and one draft cane ticket left unposted**, so
  a tester can post them and watch the stock move
- a second company with its own approved budget, so the consolidated report
  has more than one company to consolidate

It is additive, not idempotent — run it against a freshly migrated database.
It refuses to run twice unless you pass `--force`.

---

## Layout

```
backend/                 Go service
  cmd/api/               entry point: signals and the listener
  internal/app/          the dependency graph, shared with the tests
  internal/model/        GORM entities
  internal/dto/          request and response shapes
  internal/repository/   interfaces + the PostgreSQL implementations
  internal/service/      business logic and transaction boundaries
  internal/controller/   HTTP handlers
  internal/middleware/   request id, logging, recovery, CORS, authorization
  internal/router/       the REST contract, with each route's permission
  migrations/            SQL migrations, embedded in the binary
    demo/                demo data — its own source, applied only in DEV/SIT
  tests/                 integration tests against a real PostgreSQL
frontend/webapp/         SAP UI5 application
docs/openapi.yaml        the API contract
schema.sql               generated from the migrations by `make schema`
```

The layering is enforced, not just described: a controller never imports GORM,
a service depends only on repository interfaces, a repository holds no business
rule, and models never cross the controller boundary — controllers speak DTO.

---

## The decisions worth knowing

### One login, many companies, no global company

The access token identifies the user and carries **no company**. Every
company-dependent call names the company explicitly — path parameter,
`companyId` query parameter or the `X-Company-Id` header — and the server
re-validates the entitlement against `user_companies` every time. The
client-supplied company is a request parameter, never a claim of rights.

The consequence is deliberate and worth protecting: two browser tabs can work
on two companies simultaneously with no interference. The UI has **no global
company switcher**; each screen owns a Company drop-down whose selection lives
in that view's own model.

Permissions resolve per *(user, company)*. The same user is a planner in one
company and a display user in the next, so the permission cache is keyed on the
pair — never on the user alone.

### The planning matrix round-trips, and plans the whole plant at once

Planning is a Date × Line grid. Its columns are the production lines of the
selected company, generated at runtime. The save is an idempotent upsert keyed
on `(document, date, line, material, process)`, so replaying a payload produces
the same grid rather than duplicate rows.

A planner works on a week of the whole plant, not on one product at a time, so
the same endpoint takes several products in one payload:

```jsonc
POST /plans/matrix
{ "versionId": 4, "dateFrom": "…", "dateTo": "…",
  "series": [ { "materialId": …, "processId": …, "rows": [ … ] },   // raw sugar
              { "materialId": …, "processId": …, "rows": [ … ] } ] } // molasses
```

All of it lands in one transaction: a rejected cell in the fourth product rolls
the first three back rather than leaving a half-saved week. Deletion of omitted
cells stays scoped to the product that was saved, so writing raw sugar never
clears the molasses grid beside it. `GET /plans/matrix/all` returns every
product a version plans over a window, which is what the planning screen opens
on; the single-product `GET`/`POST` shape still works unchanged.

### Cane comes from somewhere

Upstream of the mill plan is the cane supply plan: which growers and which
fields deliver how much, on which day. It is a Date × Grower grid that lives
inside the *same* planning version as the production plan, so one submission,
one approval and one status machine cover both — and a version that has been
approved refuses cane edits with the same `E-PLAN-007` the production plan
uses, enforced in the service and again by a database trigger.

Own-estate cane and **purchased cane** are planned in the same grid and
reported apart, because only purchased cane carries a supply contract, a
contracted tonnage and a price per tonne. The price is copied onto the
weighbridge ticket when the load is recorded, not read through a join:
re-negotiating a contract must not rewrite what an already-delivered lorry was
worth. A load with no agreed price has an unknown value — reported as absent,
never as zero.

The actual against that plan is the weighbridge ticket. Its net weight is a
generated column, so no code path can leave gross, tare and net disagreeing,
and posting one writes an ordinary inventory movement for the net weight.
Cane is therefore not a special case anywhere in the ledger: capacity, negative
stock, the open season and the location's allowed materials all apply to it
exactly as they apply to sugar.

Two things that look alike but are not: a `null` quantity means *leave this
value alone*, while a cell the payload omits is *deleted* — unless
`partialUpdate` is set, in which case anything unmentioned is left alone.

### A copied version is genuinely independent

Copying a version duplicates every header and item in two set-based statements,
using a CTE that maps old document ids to new ones. Nothing is shared with the
source. The only link is the informational `copiedFromVersionId`, so editing
V3 can never disturb V2.

### Rules are enforced twice where it matters

The planning status machine and the no-cross-company-movement rule are checked
in the service *and* by database triggers, so no path — including a future
batch job or a manual SQL session — can bypass them. Where a trigger raises,
it raises the same error code the Go check would have returned.

### Quantities are positive; direction comes from the data

An inventory quantity is always positive. Whether it adds to or subtracts from
stock comes from `movement_types.direction`. No business logic switches on a
hard-coded movement code.

### The production routing is master data, not code

Refined and super refined sugar pass through the condition silo; white sugar
goes direct to its warehouse and never enters it. That rule is expressed as
`materials.conditioning_required` and
`process_materials.requires_conditioning`, so a third grade or a second silo is
a master-data change. Three checks enforce it at posting time:

| Situation | Refused with |
|---|---|
| material is not a declared input or output of the process | `E-PROD-020` |
| a non-conditioned grade is received into a silo | `E-PROD-021` |
| a conditioned grade is received straight into finished goods | `E-PROD-022` |

### A tank and a condition silo are warehouses, not new entities

Master data maintenance covers company, material, warehouse, condition silo and
tank. The last three are one table: a tank and a silo are warehouses whose
`warehouseType` differs, so they share the optimistic lock, the capacity rule,
the audit trail and the material restriction rather than duplicating them.
`GET /companies/{id}/tanks` and `.../condition-silos` are the type-filtered
views the maintenance screens open on, and
`PUT /companies/{id}/warehouses/{id}/materials` maintains what a location may
hold on its own — the same restriction the posting rules read.

### Nothing is deleted

Master data is deactivated. Documents are reversed: the original movement stays
in the ledger and an opposite one is written beside it. The ledger is the source
of truth; `inventory_balances` is a projection of it, maintained incrementally
under a row lock and rebuilt by a reconciliation job that reports any drift.

### Values that have no number stay empty

A location with no maintained capacity is *unlimited* — its utilisation is
`null`, not `0` and not infinity. Production against a zero plan has no
variance percentage — also `null`, rendered as "n/a", never as 0 or 100 by
accident.

### The data browser is the highest-risk feature, so

the table must be flagged browsable in the dictionary; every field named in
`fields`, `sort` or `filter` must exist on that table; values are always bound
parameters; for a company-dependent table the company predicate is injected
server-side and cannot be removed by the caller; personal data is masked; rows
and statement time are capped; and every query is written to the audit log.
`users` and `refresh_tokens` are documented but not browsable at all.

---

## Tests

```bash
make test     # everything, needs a PostgreSQL
make unit     # only the tests that need no database
make cover    # statement coverage per package
```

108 tests. The unit tests pin the pure rules — the variance formula, the status
machine, argon2id handling. The integration tests boot the real dependency
graph from `internal/app` against a real PostgreSQL, so they exercise the
production wiring including the middleware chain, the triggers and the
exclusion constraints. They cover:

- company isolation and per-company permissions
- the matrix round trip, its idempotency, and delete-versus-leave-alone
- several products planned over several days in one atomic call
- purchased versus own-estate cane, the harvest grid and its freeze on approval
- weighbridge tickets: derived net weight, posting to stock, reversal
- master data maintenance, including the tank and silo views
- copy independence, the status machine, four-eyes approval
- posting, reversal, negative stock, capacity bands, unit conversion
- the silo routing rules, including the valid conditioning route
- number-range uniqueness and format
- the browser's whitelist, field validation and injected company filter
- role revocation, token rotation, password change

The suite skips itself when no database is reachable, so `go test ./...` still
works without one. Point it elsewhere with `TEST_DB_HOST` and friends; it
creates and drops its own database per run.

Current statement coverage: **77.6 % on the service layer, 77.7 % overall**.
The specification asks for ≥ 80 % on the service layer, so this is short of
target. What remains uncovered is almost entirely database-error branches,
which need fault injection rather than more scenarios.

---

## Configuration

Demo data lives in its own migration source (`backend/migrations/demo`) with
its own version sequence and its own tracking table, so reference data can
always be added after it. A database migrated before that split records
version 3 for the retired demo migration and must be recreated.

Everything comes from the environment; see
[`backend/.env.example`](backend/.env.example). Nothing outside
`internal/config` reads `os.Getenv`.

Production refuses to start on an unsafe configuration: a default or short JWT
secret, `DB_SSLMODE=disable`, demo data enabled, or demo users enabled. Each
environment gets its own database and its own signing key.

---

## Known gaps

These are deliberate, not oversights:

- **Report export** (`?format=xlsx|csv|pdf`) is specified but not implemented.
  The reports themselves return JSON.
- **The UI5 runtime loads from a CDN.** Vendor it, or point the bootstrap in
  `frontend/index.html` at a local copy, for an environment without outbound
  access.
- **The login rate limiter is in-process.** With one instance that is enough;
  behind several, the ingress or a shared store has to take over.
- **Partitioning** of `inventory_movements` and `audit_log` is phase 2, as is
  the yield-variance report — `expected_yield_pct` is captured but not enforced.
- **Consolidated reporting is quantities only.** Currency conversion is out of
  scope, matching the open question in the specification.

Open questions from the specification were resolved as the document's own
defaults proposed — `BIGINT` keys, global materials with per-company relevance,
nullable packaging on plan items, restricted tanks and silos, four-eyes on by
default, quantities-only consolidation, optional shift, electricity in MWh and
not stock managed.
