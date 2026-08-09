# Technical Specification for Development
## Multi-Company Production Planning vs. Actual Management System

**Document version:** 1.0
**Date:** 09 August 2026
**Status:** For development kick-off
**Source:** Full Development Prompt — Multi-Company Production Planning vs Actual System (§1–§36)

---

## 0. How to read this document

This is the technical translation of the business prompt into buildable artefacts. It is organised as:

| Part | Content |
|---|---|
| A | Architecture, stack, environments |
| B | Data model (tables, keys, constraints, indexes) |
| C | Security & authorization design |
| D | Backend design (layers, DI, error model, transactions) |
| E | REST API contract |
| F | Core business algorithms |
| G | Data Browser & Data Dictionary |
| H | Frontend (SAP UI5) design |
| I | Non-functional requirements |
| J | Delivery plan, definition of done, open questions |

Companion file: `schema.sql` — executable PostgreSQL DDL for the full core model.

---

# PART A — ARCHITECTURE

## A1. Technology baseline

| Layer | Technology | Pinned version (proposed) |
|---|---|---|
| Database | PostgreSQL | 16.x |
| Backend runtime | Go | 1.22+ |
| ORM | GORM | v2 (gorm.io/gorm) |
| HTTP router | Gin (or Echo) | latest stable |
| Migrations | golang-migrate | v4 |
| Auth | JWT (access + refresh), bcrypt/argon2id hashing | — |
| Validation | go-playground/validator | v10 |
| Logging | zerolog / zap (structured JSON) | latest |
| Config | Viper + `.env` | latest |
| Testing | testify, sqlmock, testcontainers-go | latest |
| Frontend | SAP UI5 | 1.120 LTS |
| Frontend UX | SAP Fiori 3 / Horizon theme | — |
| Container | Docker + docker-compose | — |

**Decision:** REST + JSON only (no OData v4 in phase 1). UI5 consumes REST through a `JSONModel`-based service layer. If SAP integration later requires OData, an OData façade can be added over the same service layer without changing business logic.

## A2. Layering rules (enforced in code review)

```
Router → Middleware → Controller → Service → Repository(interface) → GORM → PostgreSQL
```

Hard rules:

1. Controller **never** imports `gorm.io/gorm` and never references a model's persistence concerns.
2. Service depends only on **repository interfaces**, never on concrete `postgres` implementations.
3. Repository **never** contains business rules (no authorization checks, no calculations).
4. Models (GORM structs) never cross the controller boundary — controllers speak **DTO** only.
5. `mapper` package converts Model ↔ DTO. No mapping logic inside controllers or services beyond calling the mapper.
6. Transactions are opened and committed in the **service** layer, passed to repositories as a `*gorm.DB` transaction handle or via a `UnitOfWork` abstraction.

## A3. Project structure

```
backend/
├── cmd/api/main.go                 # composition root — all wiring here
├── internal/
│   ├── config/                     # env loading, typed config struct
│   ├── database/                   # connection pool, tx helper, UnitOfWork
│   ├── model/                      # GORM entities (1 file per aggregate)
│   ├── dto/                        # request/response structs + validation tags
│   ├── repository/
│   │   ├── interfaces/             # MaterialRepository, PlanRepository, ...
│   │   └── postgres/               # concrete GORM implementations
│   ├── service/                    # business logic, transactions, orchestration
│   ├── controller/                 # HTTP handlers, DTO binding, status codes
│   ├── middleware/                 # auth, company auth, logging, recovery, CORS, rate limit
│   ├── router/                     # route registration per module
│   ├── validator/                  # custom validators (season overlap, date range, UOM)
│   ├── mapper/                     # model ↔ dto
│   ├── security/                   # JWT, password hashing, permission evaluator
│   ├── audit/                      # audit interceptor + audit_log writer
│   ├── datadictionary/             # metadata repository + browser query builder
│   └── errors/                     # AppError type, error codes, HTTP mapping
├── migrations/                     # 0001_init.up.sql / .down.sql ...
├── tests/                          # integration tests (testcontainers)
├── go.mod / go.sum / Dockerfile / .env.example
```

Frontend:

```
frontend/webapp/
├── manifest.json
├── Component.js
├── controller/            # one controller per view
├── view/                  # XML views
├── fragment/              # dialogs, value helps
├── model/                 # models.js, formatter.js, ODataless service wrapper
├── service/               # ApiClient.js (fetch wrapper, token, company header)
├── i18n/
└── css/
```

## A4. Composition root pattern (`main.go`)

```go
cfg := config.Load()
db  := database.NewDB(cfg)               // single *gorm.DB, single pool
uow := database.NewUnitOfWork(db)

// repositories
materialRepo  := postgres.NewMaterialRepository(db)
planRepo      := postgres.NewPlanRepository(db)
authzRepo     := postgres.NewAuthorizationRepository(db)
numberRepo    := postgres.NewNumberRangeRepository(db)

// services
authz         := service.NewAuthorizationService(authzRepo)
numbering     := service.NewNumberRangeService(numberRepo)
materialSvc   := service.NewMaterialService(materialRepo, authz)
planSvc       := service.NewPlanService(planRepo, numbering, authz, uow)

// controllers
materialCtl   := controller.NewMaterialController(materialSvc)
planCtl       := controller.NewPlanController(planSvc)

r := router.New(cfg, mw, materialCtl, planCtl /* ... */)
r.Run(cfg.HTTPAddr)
```

No `database.NewDB` call may appear anywhere except `main.go` (and test setup).

## A5. Environments

| Env | Purpose | Data |
|---|---|---|
| DEV | developer laptops / docker-compose | synthetic seed |
| SIT | integration testing | anonymised |
| UAT | business acceptance | copy of production structure, masked |
| PRD | production | live |

Every environment gets its own DB schema/instance and its own JWT signing key. Migrations run automatically on deploy in DEV/SIT, by controlled job in UAT/PRD.

---

# PART B — DATA MODEL

## B1. Conventions

* All tables `snake_case`, plural.
* Surrogate PK: `id BIGSERIAL` (or `UUID` — see open question OQ-1). Spec below assumes `BIGINT` identity.
* Every business table carries: `is_active BOOLEAN`, `version INTEGER` (optimistic lock), `created_by`, `created_at`, `changed_by`, `changed_at`.
* Company-dependent tables carry `company_id BIGINT NOT NULL REFERENCES companies(id)`.
* Quantities: `NUMERIC(18,3)`. Percentages: `NUMERIC(9,4)`. Capacity: `NUMERIC(18,3)`.
* Timestamps: `TIMESTAMPTZ`, set by database `now()` or Go server clock — **never** from the client (§7).
* Business dates (transaction_date, plan_date): `DATE` (no time component, no timezone drift).
* Deletion: logical only (`is_active = false`) for master data; transactional documents are reversed, never deleted.

## B2. Entity groups

```
SECURITY        companies, users, roles, permissions, role_permissions,
                user_companies, user_company_roles, refresh_tokens

MASTER          materials, company_materials, warehouses, packaging_types,
                production_lines, processes, movement_types, uoms, seasons

PLANNING        planning_versions, plan_headers, plan_items

ACTUAL          actual_headers, actual_items

INVENTORY       inventory_movements, inventory_balances (materialised)

TECHNICAL       number_ranges, audit_log,
                dd_tables, dd_fields, dd_domains, dd_value_helps
```

## B3. Key table definitions

Full DDL is in `schema.sql`. Highlights and non-obvious constraints:

### companies (§8)
`company_code VARCHAR(10) UNIQUE NOT NULL`, `local_currency CHAR(3)`, `group_currency CHAR(3)`, `timezone VARCHAR(64) NOT NULL DEFAULT 'UTC'`, `fiscal_year_variant VARCHAR(10)`.

### users / user_companies / user_company_roles (§10, §13)
* `users`: `username UNIQUE`, `email UNIQUE`, `password_hash`, `is_locked`, `failed_login_count`, `last_login_at`, `must_change_password`.
* `user_companies`: `UNIQUE(user_id, company_id)`; partial unique index guarantees **at most one default company per user**:
  `CREATE UNIQUE INDEX ux_user_default_company ON user_companies(user_id) WHERE is_default AND is_active;`
* `user_company_roles`: `UNIQUE(user_id, company_id, role_id)` — this is the table that realises "same user, different role per company".
* Explicitly forbidden: a `company_id` column on `users` (§10).

### permissions
Naming convention `<MODULE>.<OBJECT>.<ACTION>`, e.g.
`PLAN.VERSION.CREATE`, `PLAN.VERSION.APPROVE`, `ACTUAL.POST`, `INV.MOVEMENT.CREATE`, `INV.MOVEMENT.REVERSE`, `MASTER.MATERIAL.EDIT`, `REPORT.PLANACTUAL.VIEW`, `DD.MAINTAIN`, `BROWSER.VIEW`, `ADMIN.USER.MANAGE`.

### materials / company_materials (§14)
`materials` is **cross-company master** (`material_code UNIQUE`). Company relevance and usage flags live in `company_materials` with `UNIQUE(company_id, material_id)` and boolean flags `planning_enabled`, `production_enabled`, `inventory_enabled`, `sales_enabled`.
`material_type ∈ {RAW, SEMI_FINISHED, FINISHED, BY_PRODUCT, UTILITY}`.

### warehouses (§15–§17)
`UNIQUE(company_id, warehouse_code)`; `warehouse_type ∈ {WAREHOUSE, TANK, SILO, PRODUCTION_STORAGE}`; `capacity NUMERIC(18,3) NULL`, `capacity_uom_id`.
Optional `allowed_material_group` / link table `warehouse_materials` if a tank must be restricted to molasses (recommended — see OQ-4).

### seasons (§27)
`UNIQUE(company_id, season_code)`; `CHECK (end_date > start_date)`; `status ∈ {PLANNING, OPEN, CLOSED}`.
Constraint: seasons of one company must not overlap → enforced with an exclusion constraint using `daterange`:
`EXCLUDE USING gist (company_id WITH =, daterange(start_date, end_date, '[]') WITH &&) WHERE (is_active)`.

### planning_versions (§29, §30)
`UNIQUE(company_id, season_id, version_no)`; `version_no INTEGER` (open-ended, never hard-coded to 3); `status ∈ {DRAFT, SUBMITTED, APPROVED, LOCKED, CANCELLED}`; `copied_from_version_id BIGINT NULL` (lineage); `approved_by`, `approved_at`.

### plan_headers / plan_items (§28)
Header = one planning document per (company, season, version, movement_type, optionally production_line, date range).
Item = the daily granular row.

`plan_items` unique business key:
`UNIQUE (plan_header_id, plan_date, production_line_id, material_id, process_id)`
This is what makes the matrix "Date × Line" (§28 example) round-trippable.

`plan_items` columns: `plan_date DATE`, `production_line_id`, `material_id`, `process_id`, `warehouse_id NULL`, `quantity NUMERIC(18,3) CHECK (quantity >= 0)`, `uom_id`, `packaging_type_id NULL`, `remark`.

### actual_headers / actual_items (§32)
Structurally parallel to planning **but with no FK to `planning_versions`** — actuals are independent (§32). Additional dimensions: `warehouse_id`, `packaging_type_id`, `shift` (optional), `posting_status ∈ {DRAFT, POSTED, REVERSED}`.
Posting an actual document generates the corresponding `inventory_movements` in the same transaction.

### inventory_movements (§33)
Append-only ledger. Columns: `company_id, document_no, transaction_date, material_id, warehouse_id, movement_type_id, packaging_type_id, quantity, uom_id, reference_document, reference_id, source_module ∈ {ACTUAL, MANUAL, TRANSFER, OPENING, REVERSAL}, reversed_movement_id NULL`.

Sign convention: **quantity is always positive**; direction comes from `movement_types.direction ∈ {IN, OUT}`. Reporting sums `quantity * direction_sign`.

Transfers create **two** rows (TRANSFER_OUT + TRANSFER_IN) linked by `transfer_group_id`, and a check enforces both rows share the same `company_id` (§34: no cross-company transfers).

### inventory_balances
Materialised daily/current balance for performance:
`PK (company_id, warehouse_id, material_id, packaging_type_id, balance_date)` with `opening_qty, in_qty, out_qty, closing_qty`.
Maintained by service logic on posting (incremental) plus a nightly reconciliation job that recomputes from `inventory_movements` and logs discrepancies.

### number_ranges (§35)
`UNIQUE(company_id, object_type, fiscal_year)`; `prefix`, `current_no BIGINT`, `length INTEGER DEFAULT 6`.
Concurrency-safe allocation — see §F1.

### audit_log (§7)
`table_name, record_id, company_id, action ∈ {INSERT, UPDATE, DELETE, APPROVE, POST, REVERSE, LOGIN, LOGIN_FAIL}, changed_by, changed_at, old_values JSONB, new_values JSONB, request_id, ip_address`.
Written by a GORM callback/interceptor for CRUD, and explicitly by services for business events.

## B4. Indexing plan

| Table | Index |
|---|---|
| plan_items | `(plan_header_id, plan_date)`, `(material_id, plan_date)` |
| plan_headers | `(company_id, season_id, planning_version_id, movement_type_id)` |
| actual_items | `(actual_header_id, actual_date)`, `(company_id, actual_date, material_id)` |
| inventory_movements | `(company_id, transaction_date)`, `(company_id, warehouse_id, material_id, transaction_date)`, `(document_no)` |
| inventory_balances | PK covers access path; extra `(company_id, balance_date)` |
| audit_log | `(table_name, record_id)`, `(changed_at)`, `(company_id, changed_at)` BRIN on `changed_at` |
| user_company_roles | `(user_id, company_id)` |

Partitioning (phase 2): `inventory_movements` and `audit_log` by `RANGE (transaction_date / changed_at)` yearly.

---

# PART C — SECURITY & AUTHORIZATION

## C1. Session model (§9)

**One login → many authorised companies → one application → company chosen per transaction.**

* JWT access token (15 min) contains: `sub` (user id), `username`, `jti`, `iat`, `exp`.
* JWT **must not** contain a "current company". There is no server-side or client-side global company context (§9).
* Refresh token (8 h, rotating, stored hashed in `refresh_tokens`, revocable).
* Company travels **per request**, as an explicit parameter: path/query/body field `company_id`, or header `X-Company-Id` for GET reports.
* Consequence: two browser tabs can operate on two companies simultaneously with zero interference. No `sessionStorage`-based global company. UI5 stores the selection in the **view model of that view only**.

## C2. Authorization pipeline (§12)

Every company-dependent endpoint runs:

```
1. AuthMiddleware        → validate JWT signature/expiry → load user (cached)
2. Extract companyId     → from validated request payload (never trusted as authorization proof)
3. CompanyAuthMiddleware → SELECT 1 FROM user_companies WHERE user_id AND company_id AND is_active
                           ✗ → 403 E-AUTH-003 COMPANY_NOT_AUTHORIZED
4. PermissionCheck       → roles = user_company_roles(user, company)
                           perms = role_permissions(roles)
                           required permission ∈ perms ?  ✗ → 403 E-AUTH-004 PERMISSION_DENIED
5. Service executes business logic
6. Repository additionally filters WHERE company_id = :companyId  (defence in depth)
```

Rules:

* The client-supplied `company_id` is treated as a *request parameter*, never as an assertion of rights — it is always re-validated (§12).
* Every repository read for company-dependent data takes `companyId` as a mandatory argument. A GORM global scope (`CompanyScope`) is applied as a second barrier; a repository method that omits it fails code review.
* A cross-company request (e.g. plan header of company 1000, warehouse of company 2000) is rejected with `422 E-VAL-010 CROSS_COMPANY_REFERENCE`.
* 403 is returned for "authenticated but not entitled"; 401 only for missing/expired/invalid token.

## C3. Permission cache

Per-request resolution would cost 2 joins on every call. Cache `(user_id, company_id) → permission set` in an in-process TTL cache (60 s) invalidated on role/assignment change. Cache key must include company — never a per-user-only cache.

## C4. Additional controls

* Password policy: min 12 chars, argon2id, `must_change_password` on admin reset, lockout after N failed attempts.
* All state-changing endpoints require `Content-Type: application/json`, CSRF not needed for pure bearer-token APIs, but CORS is restricted to known origins.
* Rate limiting on `/auth/login` (per IP + per username).
* TLS termination at the ingress; HSTS.
* Audit: login, failed login, permission denial, approval, posting, reversal.

---

# PART D — BACKEND DESIGN DETAILS

## D1. Error model

Single `errors.AppError{ Code, Message, Details, HTTPStatus, Cause }`. Controllers translate to:

| HTTP | Use |
|---|---|
| 400 | malformed JSON / binding failure |
| 401 | no/invalid token |
| 403 | company or permission denied |
| 404 | resource not found *within the authorised company* |
| 409 | optimistic lock conflict, duplicate key, status conflict (e.g. edit of LOCKED version) |
| 422 | business rule violation (capacity exceeded, negative stock, cross-company reference) |
| 500 | unexpected |

Response envelope:

```json
{ "success": false,
  "error": { "code": "E-PLAN-007",
             "message": "Planning version is LOCKED and cannot be modified",
             "details": [{"field": "versionId", "value": "42"}] },
  "requestId": "b2f1..." }
```

Success envelope:

```json
{ "success": true, "data": { ... }, "meta": { "page":1, "size":50, "total":1234 } }
```

## D2. Optimistic locking

`version INTEGER NOT NULL DEFAULT 1`. Update statement:
`UPDATE ... SET version = version + 1 WHERE id = ? AND version = ?` → 0 rows affected ⇒ `409 E-GEN-409 STALE_RECORD`. UI5 shows "Record was changed by another user, please reload".

## D3. Transaction boundaries

Managed by `UnitOfWork` in the service layer. Operations that **must** be a single transaction:

* Posting an actual document: header + items + inventory movements + balance update + number assignment + audit.
* Copying a planning version: new version row + all headers + all items (§31).
* Warehouse transfer: OUT movement + IN movement + both balances.
* Reversal: reversal movements + status flip on the original.

## D4. Audit interceptor (§7)

GORM `Before/AfterCreate|Update` hooks:
* set `created_by/changed_by` from `ctx.Value(UserIDKey)`;
* set `created_at/changed_at = time.Now().UTC()` on the server;
* reject any client-supplied value in those four fields at the DTO layer (they are simply absent from request DTOs).
Write `audit_log` row with JSONB diff.

## D5. Logging & observability

Structured JSON logs with `request_id`, `user_id`, `company_id`, `route`, `latency_ms`, `status`. `/healthz`, `/readyz`. Prometheus metrics: request duration histogram, DB pool stats, posting counters. Slow-query log > 500 ms.

---

# PART E — REST API CONTRACT

Base path: `/api/v1`. All company-dependent routes carry the company explicitly.

## E1. Auth
```
POST   /auth/login                     → tokens + user profile + authorised companies
POST   /auth/refresh
POST   /auth/logout
GET    /auth/me                        → user + companies + per-company roles/permissions
POST   /auth/change-password
```
`GET /auth/me` response drives the UI5 company dropdown (§11) — it returns only authorised companies and marks the default one.

## E2. Master data
```
GET|POST        /companies                 GET|PUT|DELETE /companies/{id}
GET|POST        /materials                 GET|PUT|DELETE /materials/{id}
GET|POST|PUT    /companies/{companyId}/materials          (company_materials)
GET|POST        /companies/{companyId}/warehouses
GET             /companies/{companyId}/warehouses/{id}/capacity
GET|POST        /packaging-types
GET|POST        /companies/{companyId}/production-lines
GET|POST        /companies/{companyId}/seasons
GET|POST        /movement-types            GET|POST /processes            GET|POST /uoms
```

## E3. Planning
```
GET|POST   /companies/{companyId}/seasons/{seasonId}/planning-versions
GET        /planning-versions/{id}
PUT        /planning-versions/{id}
POST       /planning-versions/{id}/copy         { newVersionNo, versionName, description }
POST       /planning-versions/{id}/submit
POST       /planning-versions/{id}/approve
POST       /planning-versions/{id}/lock
POST       /planning-versions/{id}/cancel

GET|POST   /plans                                ?companyId&seasonId&versionId&movementTypeId
GET|PUT    /plans/{headerId}
GET        /plans/{headerId}/items
PUT        /plans/{headerId}/items               bulk upsert of the Date × Line matrix
GET        /plans/matrix                         ?companyId&versionId&movementTypeId&dateFrom&dateTo
POST       /plans/matrix                         matrix-shaped bulk save
```

The **matrix endpoints** are the technical answer to §28 ("form planning by movement type across multiple production lines and multiple dates"). Payload:

```json
{ "companyId": 1000, "seasonId": 7, "versionId": 42,
  "movementTypeId": 3, "materialId": 11, "processId": 2,
  "dateFrom": "2026-11-01", "dateTo": "2026-11-03",
  "lines": [{"productionLineId": 1, "code": "LINE1"}, ...],
  "rows": [
    { "planDate": "2026-11-01", "values": [{"productionLineId":1,"quantity":5000}, {"productionLineId":2,"quantity":3000}] },
    { "planDate": "2026-11-02", "values": [ ... ] }
  ] }
```

Server performs an idempotent upsert keyed on `(header, date, line, material, process)`; missing cells are treated as delete, `null` as "leave unchanged" (flag `partialUpdate`).

## E4. Actual
```
GET|POST   /actuals                     ?companyId&dateFrom&dateTo&movementTypeId&lineId
GET|PUT    /actuals/{headerId}
POST       /actuals/{headerId}/post
POST       /actuals/{headerId}/reverse
GET|PUT    /actuals/{headerId}/items
```

## E5. Inventory
```
GET|POST   /inventory/movements          ?companyId&warehouseId&materialId&dateFrom&dateTo
POST       /inventory/transfers          { fromWarehouseId, toWarehouseId, materialId, qty, ... }
POST       /inventory/adjustments
POST       /inventory/movements/{id}/reverse
GET        /inventory/balances           ?companyId&warehouseId&materialId&asOfDate
GET        /inventory/capacity           ?companyId&warehouseId
```

## E6. Reporting
```
GET /reports/plan-vs-actual              ?companyId&seasonId&versionId|latestApproved=true
                                         &dateFrom&dateTo&groupBy=material|line|process|date
GET /reports/plan-vs-actual/consolidated ?companyIds=1000,2000,3000&seasonId&...
GET /reports/production-summary
GET /reports/inventory-movement
GET /reports/capacity-utilisation
GET /reports/{name}/export               ?format=xlsx|csv|pdf
```
Consolidated reports (§1) validate **every** company id in the list against `user_companies`; unauthorised ids cause 403, not silent filtering (explicit is safer for an audit trail — see OQ-6).

## E7. Data Browser & Data Dictionary
```
GET  /dd/tables                          ?module&search
GET  /dd/tables/{tableName}
GET  /dd/tables/{tableName}/fields
PUT  /dd/tables/{tableName}              (maintain business description; permission DD.MAINTAIN)
PUT  /dd/fields/{id}
GET  /dd/domains
GET  /browser/{tableName}                ?companyId&filters&fields&sort&page&size
GET  /browser/{tableName}/export
```

## E8. Cross-cutting conventions

* Pagination: `page` (1-based), `size` (default 50, max 500), response `meta.total`.
* Sorting: `sort=field,-field2`.
* Filtering: `filter[field]=op:value` with ops `eq,ne,gt,ge,lt,le,like,in,between`.
* Idempotency: `Idempotency-Key` header honoured on POST of documents to prevent double posting on retry.
* All list endpoints return only rows of authorised companies.

---

# PART F — CORE BUSINESS ALGORITHMS

## F1. Document number generation (§35) — concurrency safe

Format: `{PREFIX}-{COMPANY_CODE}-{YEAR}-{SEQ:6}` e.g. `PLAN-1000-2026-000001`.

Implementation (inside the caller's transaction):

```sql
UPDATE number_ranges
   SET current_no = current_no + 1, changed_at = now()
 WHERE company_id = $1 AND object_type = $2 AND fiscal_year = $3
RETURNING current_no;
```

The row-level lock held by `UPDATE ... RETURNING` serialises concurrent allocators. Do **not** use `SELECT` then `UPDATE`, and do **not** use a plain PostgreSQL sequence (it cannot be scoped per company/year and gaps are uncontrolled). If throughput ever becomes an issue, switch to a buffered block allocation (`current_no + N`) held in memory, accepting gaps.

If the range row is missing, auto-create it with `current_no = 0` (`INSERT ... ON CONFLICT DO NOTHING` then retry).

## F2. Copy planning version (§31)

```
BEGIN
  source = load version (must be same company + season)
  guard: target version_no must not exist for (company, season)
  new = insert planning_versions { version_no, name, description,
                                   status = DRAFT, copied_from_version_id = source.id }
  INSERT INTO plan_headers (…) SELECT … , new.id FROM plan_headers WHERE version = source.id
  INSERT INTO plan_items   (…) SELECT … , mapped_header_id FROM plan_items …
COMMIT
```

Header id mapping is done with a CTE returning `(old_id, new_id)` so items can be inserted in one statement. Result is a **deep, independent snapshot** — no shared rows, no FK from V3 back to V2 other than the informational `copied_from_version_id`. Editing V3 can never touch V2 (§31).

## F3. Version status guard (§30)

State machine:

```
DRAFT ──submit──► SUBMITTED ──approve──► APPROVED ──lock──► LOCKED
  │                   │                      │
  └────cancel─────────┴──────cancel──────────┘ ──► CANCELLED
```

* Mutation of `plan_headers` / `plan_items` allowed only when version status = `DRAFT` (or `SUBMITTED` with permission `PLAN.EDIT.SUBMITTED`).
* `APPROVED`/`LOCKED` → any write returns `409 E-PLAN-007`.
* Enforced twice: in `PlanService.assertEditable()` **and** by a DB trigger `trg_plan_items_status_guard` so no path (including a future batch job) can bypass it.
* Transition permissions: `PLAN.VERSION.SUBMIT`, `.APPROVE`, `.LOCK`, `.CANCEL`. Approver ≠ creator if `enforce_four_eyes` config is on (OQ-5).

## F4. Inventory balance (§34)

```
Closing = Opening
        + PRODUCTION_RECEIPT + TRANSFER_IN + ADJUSTMENT_IN
        - PRODUCTION_ISSUE  - REMELT_ISSUE - TRANSFER_OUT - SALE - ADJUSTMENT_OUT
```

Driven by data, not by hard-coded movement codes: `movement_types.direction (IN|OUT)` and `movement_types.affects_stock (bool)`.

Posting validations:
1. Warehouse and material belong to the same company as the document (§34).
2. `company_materials.inventory_enabled = true`.
3. Resulting stock ≥ 0 unless `allow_negative_stock` is set on the warehouse.
4. Resulting stock ≤ `warehouses.capacity` when capacity is maintained (§17) → `422 E-INV-012 CAPACITY_EXCEEDED`.
5. Transfer: `from.company_id = to.company_id`, `from ≠ to`.
6. Transaction date must fall inside an open season/period and not be in the future beyond `n` days.

Concurrency: `SELECT ... FOR UPDATE` on the `inventory_balances` row for `(company, warehouse, material, packaging)` before applying the delta, guaranteeing serialisation without table-level locks.

Reversal: never delete a movement; insert an opposite movement with `reversed_movement_id` set and mark the original `is_reversed = true`.

## F5. Capacity utilisation (§17)

```
available    = capacity - current_stock
utilisation% = capacity > 0 ? current_stock / capacity * 100 : NULL
```
`capacity = NULL` ⇒ unlimited: no check, utilisation reported as `null` (not 0, not ∞). Traffic-light thresholds (configurable): < 70 % green, 70–90 % amber, > 90 % red.

## F6. Plan vs Actual (§36)

```
variance   = actual - plan
variance % = plan <> 0            ? (actual - plan) / abs(plan) * 100
           : actual = 0          ? 0
           : NULL   (rendered as "n/a" or "new", never ÷0, never 100 by accident)
```

`abs(plan)` in the denominator keeps the sign of the variance meaningful if a plan value is ever negative.

Query shape — full outer join so that plan-only and actual-only rows both appear:

```sql
WITH plan AS (
  SELECT pi.material_id, pi.production_line_id, SUM(pi.quantity) qty
  FROM plan_items pi JOIN plan_headers ph ON ph.id = pi.plan_header_id
  WHERE ph.company_id = :c AND ph.planning_version_id = :v
    AND pi.plan_date BETWEEN :from AND :to
  GROUP BY 1,2),
act AS (
  SELECT ai.material_id, ai.production_line_id, SUM(ai.quantity) qty
  FROM actual_items ai JOIN actual_headers ah ON ah.id = ai.actual_header_id
  WHERE ah.company_id = :c AND ah.posting_status = 'POSTED'
    AND ai.actual_date BETWEEN :from AND :to
  GROUP BY 1,2)
SELECT COALESCE(p.material_id, a.material_id) material_id,
       COALESCE(p.qty,0) plan_qty, COALESCE(a.qty,0) actual_qty,
       COALESCE(a.qty,0) - COALESCE(p.qty,0) variance
FROM plan p FULL OUTER JOIN act a USING (material_id, production_line_id);
```

"Latest approved version" resolves to
`SELECT id FROM planning_versions WHERE company_id AND season_id AND status IN ('APPROVED','LOCKED') ORDER BY version_no DESC LIMIT 1`.

UOM rule: plan and actual are comparable only in the same base UOM; the service converts to `materials.base_uom` before comparing and flags rows where conversion is missing.

## F7. Production process validation (§19–§26)

The flow (cane → raw sugar / molasses / bagasse → electricity; raw sugar → remelt → refined → silo → finished goods) is modelled as **data**, not code, in `processes` + `process_materials`:

| process_code | input material | output materials |
|---|---|---|
| MILLING | Sugarcane | Raw Sugar, Molasses, Bagasse |
| POWER | Bagasse | Electricity |
| REMELT | Raw Sugar | Remelt Liquor |
| REFINING | Remelt Liquor | Refined Sugar, Super Refined Sugar, White Sugar |
| CONDITIONING | Refined Sugar, Super Refined Sugar | Refined Sugar, Super Refined Sugar (conditioned) |

**Routing rule (confirmed by business):** only **Refined Sugar** and **Super Refined Sugar** pass through the Condition Silo. **White Sugar goes direct** from refining to its warehouse and never touches the silo.

Modelled as data, not code, via a new column `process_materials.requires_conditioning BOOLEAN` (or equivalently `materials.conditioning_required`):

* `Refined Sugar` → `requires_conditioning = true`
* `Super Refined Sugar` → `requires_conditioning = true`
* `White Sugar` → `requires_conditioning = false`

Validation on posting:
1. The material of an item must be a declared input/output of the chosen process, otherwise `422 E-PROD-020 MATERIAL_NOT_VALID_FOR_PROCESS`.
2. A `PRODUCTION_RECEIPT` into a warehouse of type `SILO` is rejected for a material with `requires_conditioning = false` → `422 E-PROD-021 MATERIAL_NOT_SILO_MANAGED` (prevents White Sugar being posted into the Condition Silo).
3. A conditioned material may only reach its finished-goods warehouse via a `PRODUCTION_ISSUE` from the silo; a direct receipt from refining is rejected → `422 E-PROD-022 CONDITIONING_STEP_MISSING`.

This keeps the two routes auditable and makes a future third route (another conditioned grade, or a second silo) a master-data change rather than a code change. Yield ratios (e.g. cane → sugar %) are stored as optional `expected_yield_pct` for a future yield-variance report, not enforced in phase 1.

---

# PART G — DATA BROWSER & DATA DICTIONARY

## G1. Data Dictionary (SAP-like metadata repository)

Tables:

* `dd_domains` — technical type + length + decimals + value list (e.g. `DOM_QUANTITY`, `DOM_MOVEMENT_TYPE`).
* `dd_tables` — `table_name`, `module`, `description_en/local`, `table_type ∈ {MASTER, TRANSACTION, CUSTOMIZING, TECHNICAL}`, `is_company_dependent`, `is_browsable`.
* `dd_fields` — `table_id`, `field_name`, `position`, `label_short/medium/long`, `domain_id`, `data_type`, `length`, `decimals`, `is_key`, `is_required`, `is_pii`, `value_help_id`, `business_description`.
* `dd_value_helps` — either a fixed value list or a reference to a master table + key/text fields, used to render F4-style value help in the browser.

Seeding: a generator reads `information_schema.columns` and creates/updates `dd_tables` / `dd_fields` rows on migration, preserving human-maintained descriptions. A CI check fails the build if a new physical column has no dictionary entry — this keeps the dictionary from rotting.

## G2. Data Browser (SAP SE16-like)

* Screen 1: table selection (filter by module/type, only `is_browsable`).
* Screen 2: selection criteria generated from `dd_fields` (key fields first, ranges for dates/numbers, value help from `dd_value_helps`).
* Screen 3: result grid with column labels taken from `dd_fields.label_medium`, personalisation, export to Excel/CSV.

Security — the browser is the highest-risk feature, so:

1. Requires permission `BROWSER.VIEW`; export requires `BROWSER.EXPORT`.
2. Table name is validated against a **whitelist** from `dd_tables` (`is_browsable = true`). Never interpolate a client string into SQL.
3. Field names in `fields`/`sort`/`filter` are validated against `dd_fields` of that table.
4. All values are bound parameters — the query builder emits `$1..$n` only.
5. If `dd_tables.is_company_dependent`, an authorised `company_id` filter is **injected server-side and cannot be removed** by the caller.
6. Sensitive tables (`users`, `refresh_tokens`, `audit_log` payloads) are either not browsable or have `is_pii` fields masked.
7. Hard row cap (default 10 000, export 100 000) and statement timeout of 30 s.
8. Every browser query is written to `audit_log` with the table, filters, and row count.

---

# PART H — FRONTEND (SAP UI5)

## H1. Application shell

Fiori Launchpad-style tile home, `sap.m.Shell` + `sap.f.FlexibleColumnLayout` where useful. Routing via `manifest.json`. Module structure mirrors the backend: Dashboard, Master Data, Planning, Actual, Inventory, Reports, Data Browser, Data Dictionary, Administration.

## H2. Company handling in the UI (§9, §11)

* **No** global company switcher in the shell header. This is deliberate and must be defended in design reviews.
* Every company-dependent view has a mandatory `Company` `ComboBox` in its filter bar, filled from `/auth/me`, defaulted to `isDefault` but freely changeable.
* The selection lives in the **view model** of that view (`this.getView().getModel("view")`), not in a component-level or browser-storage model. This is what makes tab 1 = company 1000 and tab 2 = company 2000 possible without interference.
* Every API call from that view sends its own `companyId`.

## H3. Key screens

| Screen | Control | Notes |
|---|---|---|
| Planning matrix (§28) | `sap.ui.table.Table` with dynamically generated columns (one per production line) | rows = dates; editable `Input` with `type="Number"`; column set built at runtime from the production-lines call; row/column totals; paste-from-Excel support |
| Version management | `sap.m.Table` + object status for DRAFT/APPROVED/LOCKED | Copy dialog, Approve/Lock actions guarded by permissions returned from `/auth/me` |
| Actual entry | Filter bar + editable table | Post / Reverse buttons, `MessageStrip` for validation results |
| Inventory | Balance table + capacity `ProgressIndicator` | traffic-light colours from §F5 |
| Plan vs Actual | `sap.viz` column chart + variance table | conditional formatting on variance %, drilldown by material/line/date |
| Data Browser | Generated `SmartFilterBar`-like fragment + result table | fully metadata-driven from `/dd/*` |
| Data Dictionary | Master–detail | table list → field list, editable description for `DD.MAINTAIN` |

## H4. Technical conventions

* `ApiClient.js`: single fetch wrapper adding `Authorization`, `X-Request-Id`, JSON handling, 401→refresh→retry once, 403→`MessageBox` with the error code, global busy indicator.
* All texts in `i18n.properties` (English + local language). No hard-coded strings in XML views.
* Formatters in `model/formatter.js` (quantities with 3 decimals and thousands separator, variance colouring, status states).
* UI-level validation mirrors server validation but is never the only line of defence.
* Fiori guidelines: message handling via `MessageManager`, draft-like busy states, semantic colours only for semantic meaning.

---

# PART I — NON-FUNCTIONAL REQUIREMENTS

| Area | Requirement |
|---|---|
| Performance | List endpoints P95 < 500 ms at 100 k rows; plan-vs-actual report P95 < 3 s for one season; matrix save of 31 days × 5 lines < 1 s |
| Volume assumption | ~3 companies, ~5 lines, ~50 materials, ~200 days/season → ~150 k plan items/season, ~500 k inventory movements/year. Design headroom ×10 |
| Concurrency | 100 concurrent users, 20 concurrent posting users without duplicate document numbers |
| Availability | 99.5 % business hours; RPO 15 min (WAL archiving), RTO 4 h |
| Backup | Nightly full + continuous WAL; quarterly restore test |
| Security | OWASP ASVS L2; no SQL string concatenation anywhere; dependency scanning in CI |
| Auditability | Every business document change traceable to user, timestamp, before/after values; audit records immutable (revoke UPDATE/DELETE on `audit_log` from the app role) |
| Localisation | UTC storage, company timezone for display; number/date formats per user locale |
| Test coverage | ≥ 80 % on service layer; every business rule in §F has at least one test |
| Documentation | OpenAPI 3 spec generated from code, kept in the repo; ERD; runbook |

---

# PART J — DELIVERY PLAN

## J1. Phases

| Sprint | Scope | Exit criteria |
|---|---|---|
| 0 | Repo setup, CI/CD, Docker, config, logging, error model, migration framework, skeleton layers + DI | `/healthz` green in SIT |
| 1 | Security: companies, users, roles, permissions, user_companies, user_company_roles, JWT, auth + company + permission middleware, audit interceptor | A user with different roles in 3 companies is provably confined; 403 tests pass |
| 2 | Master data: materials, company_materials, warehouses (+types/capacity), packaging, production lines, processes, movement types, UOM, seasons + UI5 CRUD screens | Full master data maintainable end-to-end |
| 3 | Planning: versions, status machine, plan headers/items, matrix API, copy version, number ranges | Copy V1→V2→V3 proven independent; locked version rejects writes |
| 4 | Actual: headers/items, posting, reversal, integration with number ranges | Actuals post and reverse cleanly, independent of planning |
| 5 | Inventory: movements, transfers, adjustments, balances, capacity checks, reconciliation job | Balance formula reconciles to the ledger for a generated 1-season dataset |
| 6 | Reporting: plan vs actual (single + consolidated), capacity, movement report, exports | Variance figures match a manually calculated reference set |
| 7 | Data Dictionary + Data Browser | Browser whitelist/injection tests pass; dictionary generator wired into CI |
| 8 | Hardening: performance tuning, pen-test fixes, UAT defects, documentation, go-live runbook | UAT sign-off |

## J2. Definition of Done (per story)

1. Migration written with a working `down`.
2. Model, DTO, repository interface + implementation, service, controller, route registered via DI.
3. Company authorization and permission enforced and covered by a negative test.
4. Audit fields populated from the server clock and authenticated user.
5. Unit tests + at least one integration test against a real PostgreSQL (testcontainers).
6. OpenAPI updated; data dictionary entries present for new tables/fields.
7. UI5 screen with i18n texts, error handling, and busy state.
8. Code review confirming the layering rules of §A2.

## J3. Open questions to confirm before Sprint 1

| # | Question | Impact | Proposed default |
|---|---|---|---|
| OQ-1 | `BIGINT` identity vs `UUID` primary keys? | Migration cost later, SAP integration | BIGINT (smaller indexes, human-readable in the browser) |
| OQ-2 | Is `materials` truly global, or should material codes differ per company? | Master data model | Global material, company relevance via `company_materials` (§14 as written) |
| OQ-3 | Is packaging relevant for planning, or actual/inventory only? | `plan_items.packaging_type_id` nullable or absent | Nullable, used mainly in actual/inventory |
| OQ-4 | Should tanks/silos be restricted to specific materials? | `warehouse_materials` table | Yes, recommended (prevents posting sugar into a molasses tank) |
| OQ-5 | Four-eyes principle on version approval (approver ≠ creator)? | Approval logic | Configurable, default on |
| OQ-6 | Consolidated reports across companies: convert to `group_currency`/common UOM, or report quantities only? | Report engine | Quantities only in phase 1; currency out of scope |
| OQ-7 | Are shifts required on actual production? | `actual_items.shift` | Optional field, unused in phase 1 |
| OQ-8 | Electricity in kWh or MWh, and is it stock-managed or only generated/exported? | UOM + inventory relevance | MWh, tracked as production quantity, not warehouse stock |
| OQ-9 | Retention period for `audit_log` and browser query logs? | Partitioning/archiving | 7 years, yearly partitions |
| OQ-10 | Sections beyond §36 (Data Browser and Data Dictionary detail, integration, deployment) — were they cut from the source document? | Scope confirmation | Part G above is a proposal pending your detail |

---

## Appendix — traceability

| Prompt § | Covered in |
|---|---|
| 1–5 | Part A |
| 6–8 | B1–B3, `schema.sql` |
| 9–13 | Part C |
| 14–18 | B3, `schema.sql` |
| 19–26 | F7, master data seeds |
| 27–31 | B3, F2, F3 |
| 32 | B3 (actuals independent of planning) |
| 33–34 | B3, F4 |
| 35 | F1 |
| 36 | F6 |
| Data Browser / Dictionary | Part G |
