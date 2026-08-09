// Package app is the composition root's reusable half: it wires every layer
// together and hands back a ready HTTP handler. main.go adds process concerns
// (signals, listener) and the integration tests boot the very same graph, so
// what the tests exercise is the production wiring rather than a stand-in.
package app

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"gorm.io/gorm"

	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/audit"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/config"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/controller"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/database"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/middleware"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/repository/postgres"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/router"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/security"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/service"
)

// App is the assembled application.
type App struct {
	Config *config.Config
	Log    zerolog.Logger
	DB     *gorm.DB
	Engine *gin.Engine
}

// New builds the whole dependency graph. Per §A4 this is the only place —
// besides test setup — that opens a database connection.
func New(ctx context.Context, cfg *config.Config, log zerolog.Logger) (*App, error) {
	if cfg.DB.AutoMigrate {
		if err := database.RunMigrations(cfg, log); err != nil {
			return nil, err
		}
	}

	db, err := database.NewDB(cfg, log)
	if err != nil {
		return nil, err
	}
	uow := database.NewUnitOfWork(db)

	// --- repositories ----------------------------------------------------

	companyRepo := postgres.NewCompanyRepository(db)
	userRepo := postgres.NewUserRepository(db)
	authzRepo := postgres.NewAuthorizationRepository(db)
	tokenRepo := postgres.NewRefreshTokenRepository(db)
	auditRepo := postgres.NewAuditRepository(db)

	materialRepo := postgres.NewMaterialRepository(db)
	warehouseRepo := postgres.NewWarehouseRepository(db)
	lineRepo := postgres.NewProductionLineRepository(db)
	seasonRepo := postgres.NewSeasonRepository(db)
	processRepo := postgres.NewProcessRepository(db)
	movementRepo := postgres.NewMovementTypeRepository(db)
	uomRepo := postgres.NewUOMRepository(db)
	packagingRepo := postgres.NewPackagingTypeRepository(db)

	versionRepo := postgres.NewPlanningVersionRepository(db)
	planRepo := postgres.NewPlanRepository(db)
	actualRepo := postgres.NewActualRepository(db)
	inventoryRepo := postgres.NewInventoryRepository(db)
	numberRepo := postgres.NewNumberRangeRepository(db)
	reportRepo := postgres.NewReportRepository(db)
	ddRepo := postgres.NewDataDictionaryRepository(db)
	browserRepo := postgres.NewBrowserRepository(db)

	// --- security primitives ---------------------------------------------

	issuer := security.NewTokenIssuer(cfg.JWT)
	hasher := security.NewPasswordHasher(cfg.Security.PasswordMinLength)
	limiter := middleware.NewRateLimiter(cfg.Security.LoginRateLimit, cfg.Security.LoginRateWindow)

	// --- services --------------------------------------------------------

	auditSvc := audit.NewService(auditRepo, log)
	authz := service.NewAuthorizationService(authzRepo, cfg.Security.PermissionCacheTTL)
	numbering := service.NewNumberRangeService(numberRepo, companyRepo)
	refs := service.NewReferenceValidator(warehouseRepo, lineRepo, seasonRepo, materialRepo)

	authSvc := service.NewAuthService(userRepo, tokenRepo, authz, issuer, hasher,
		auditSvc, uow, cfg.Security)
	userSvc := service.NewUserService(userRepo, authz, hasher, auditSvc, uow)
	masterSvc := service.NewMasterDataService(companyRepo, materialRepo, warehouseRepo,
		lineRepo, seasonRepo, processRepo, movementRepo, uomRepo, packagingRepo, auditSvc)
	inventorySvc := service.NewInventoryService(inventoryRepo, warehouseRepo, materialRepo,
		movementRepo, processRepo, seasonRepo, uomRepo, numbering, refs, auditSvc, uow, cfg.Business)
	planningSvc := service.NewPlanningService(versionRepo, planRepo, lineRepo, numbering,
		authz, refs, auditSvc, uow, cfg.Business)
	actualSvc := service.NewActualService(actualRepo, inventorySvc, numbering, refs,
		seasonRepo, auditSvc, uow)
	reportSvc := service.NewReportService(reportRepo, versionRepo)
	ddSvc := service.NewDataDictionaryService(ddRepo, browserRepo, auditSvc, cfg.Business)

	// --- start-up tasks --------------------------------------------------

	// The dictionary is regenerated from the live catalogue on every start, so
	// a newly migrated table is documented immediately (§G1).
	synced, err := ddSvc.Sync(ctx)
	if err != nil {
		return nil, err
	}
	log.Info().Int("fields", synced).Msg("data dictionary synchronised")

	bootstrap := service.NewBootstrapService(userSvc, userRepo, companyRepo, authz, cfg, log)
	if err := bootstrap.Run(ctx); err != nil {
		return nil, err
	}

	// --- HTTP ------------------------------------------------------------

	engine := router.New(router.Dependencies{
		Config:  cfg,
		Log:     log,
		Issuer:  issuer,
		Authz:   authz,
		Audit:   auditSvc,
		Limiter: limiter,
		Health:  healthCheck(db),

		Auth:      controller.NewAuthController(authSvc, limiter),
		Master:    controller.NewMasterDataController(masterSvc),
		Planning:  controller.NewPlanningController(planningSvc),
		Actual:    controller.NewActualController(actualSvc),
		Inventory: controller.NewInventoryController(inventorySvc),
		Reports:   controller.NewReportController(reportSvc, authz, cfg.Business),
		Dict:      controller.NewDataDictionaryController(ddSvc, authz),
		Admin:     controller.NewAdminController(userSvc, authz, numbering, auditSvc),
	})

	return &App{Config: cfg, Log: log, DB: db, Engine: engine}, nil
}

// Server wraps the engine with the timeouts a public listener needs.
func (a *App) Server() *http.Server {
	return &http.Server{
		Addr:              a.Config.HTTPAddr,
		Handler:           a.Engine,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       90 * time.Second,
	}
}

// Close releases the connection pool.
func (a *App) Close() error {
	sqlDB, err := a.DB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func healthCheck(db *gorm.DB) func() error {
	return func() error {
		sqlDB, err := db.DB()
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		return sqlDB.PingContext(ctx)
	}
}
