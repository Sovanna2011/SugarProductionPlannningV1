// Package router registers the REST contract of Part E. Every company-
// dependent route is wrapped in the authorization chain of §C2, and the
// required permission is declared next to the route it protects.
package router

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"

	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/audit"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/config"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/controller"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/middleware"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/security"
	"github.com/Sovanna2011/SugarProductionPlannningV1/backend/internal/service"
)

// Dependencies is everything main.go wires up. Passing one struct keeps the
// composition root readable as the API grows.
type Dependencies struct {
	Config  *config.Config
	Log     zerolog.Logger
	Issuer  *security.TokenIssuer
	Authz   *service.AuthorizationService
	Audit   *audit.Service
	Limiter *middleware.RateLimiter
	Health  func() error

	Auth      *controller.AuthController
	Master    *controller.MasterDataController
	Planning  *controller.PlanningController
	Actual    *controller.ActualController
	Inventory *controller.InventoryController
	Cane      *controller.CaneController
	Reports   *controller.ReportController
	Dict      *controller.DataDictionaryController
	Admin     *controller.AdminController
}

// New builds the engine with the full middleware stack.
func New(deps Dependencies) *gin.Engine {
	if deps.Config.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}

	engine := gin.New()
	engine.Use(
		middleware.RequestID(),
		middleware.Recovery(deps.Log),
		middleware.Logger(deps.Log),
		middleware.CORS(deps.Config.CORS),
		middleware.RequireJSON(),
	)

	registerHealth(engine, deps)

	api := engine.Group("/api/v1")
	registerAuth(api, deps)

	// Everything below requires a valid token.
	secured := api.Group("")
	secured.Use(middleware.Auth(deps.Issuer))

	registerCrossCompanyMaster(secured, deps)
	registerCompanyMaster(secured, deps)
	registerMaintenance(secured, deps)
	registerPlanning(secured, deps)
	registerActual(secured, deps)
	registerInventory(secured, deps)
	registerCane(secured, deps)
	registerReports(secured, deps)
	registerDataDictionary(secured, deps)
	registerAdmin(secured, deps)

	engine.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"error":   gin.H{"code": "E-GEN-404", "message": "No such endpoint"},
		})
	})

	return engine
}

func registerHealth(engine *gin.Engine, deps Dependencies) {
	engine.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "env": deps.Config.Env})
	})
	// readyz additionally proves the database is reachable, which is what a
	// load balancer should gate traffic on.
	engine.GET("/readyz", func(c *gin.Context) {
		if err := deps.Health(); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	})
}

func registerAuth(api *gin.RouterGroup, deps Dependencies) {
	auth := api.Group("/auth")
	auth.POST("/login", middleware.LoginRateLimit(deps.Limiter), deps.Auth.Login)
	auth.POST("/refresh", deps.Auth.Refresh)

	authenticated := auth.Group("")
	authenticated.Use(middleware.Auth(deps.Issuer))
	authenticated.POST("/logout", deps.Auth.Logout)
	authenticated.GET("/me", deps.Auth.Me)
	authenticated.POST("/change-password", deps.Auth.ChangePassword)
}

// company wraps a handler in steps 3 and 4 of the authorization pipeline: the
// company named by the request is validated, then the permission is resolved
// inside that company.
func company(deps Dependencies, permission string, handler gin.HandlerFunc) []gin.HandlerFunc {
	return []gin.HandlerFunc{
		middleware.CompanyAuth(deps.Authz, deps.Audit),
		middleware.RequirePermission(deps.Authz, deps.Audit, permission),
		handler,
	}
}

// anyCompany guards a cross-company endpoint: the caller must hold the
// permission in at least one company they are authorised for.
func anyCompany(deps Dependencies, permission string, handler gin.HandlerFunc) []gin.HandlerFunc {
	return []gin.HandlerFunc{
		middleware.RequireAnyCompanyPermission(deps.Authz, deps.Audit, permission),
		handler,
	}
}

// --- master data (§E2) ---------------------------------------------------

// Cross-company master data is guarded by "holds the permission somewhere",
// because the records themselves belong to no single company (§14).
func registerCrossCompanyMaster(api *gin.RouterGroup, deps Dependencies) {
	api.GET("/materials", anyCompany(deps, "MASTER.MATERIAL.VIEW", deps.Master.ListMaterials)...)

	materials := api.Group("/materials")
	materials.GET("/:id", anyCompany(deps, "MASTER.MATERIAL.VIEW", deps.Master.GetMaterial)...)
	materials.POST("", anyCompany(deps, service.PermMaterialEdit, deps.Master.CreateMaterial)...)
	materials.PUT("/:id", anyCompany(deps, service.PermMaterialEdit, deps.Master.UpdateMaterial)...)
	materials.DELETE("/:id", anyCompany(deps, service.PermMaterialEdit, deps.Master.DeleteMaterial)...)

	api.GET("/processes", anyCompany(deps, "MASTER.PROCESS.VIEW", deps.Master.ListProcesses)...)
	api.GET("/processes/:id", anyCompany(deps, "MASTER.PROCESS.VIEW", deps.Master.GetProcess)...)
	api.POST("/processes", anyCompany(deps, service.PermProcessEdit, deps.Master.CreateProcess)...)
	api.PUT("/processes/:id/materials", anyCompany(deps, service.PermProcessEdit, deps.Master.SetProcessMaterials)...)

	api.GET("/movement-types", anyCompany(deps, "MASTER.MOVEMENTTYPE.VIEW", deps.Master.ListMovementTypes)...)
	api.POST("/movement-types", anyCompany(deps, service.PermMovementTypeEdit, deps.Master.CreateMovementType)...)

	api.GET("/uoms", anyCompany(deps, "MASTER.UOM.VIEW", deps.Master.ListUOMs)...)
	api.POST("/uoms", anyCompany(deps, service.PermUOMEdit, deps.Master.CreateUOM)...)

	api.GET("/packaging-types", anyCompany(deps, "MASTER.PACKAGING.VIEW", deps.Master.ListPackagingTypes)...)
	api.POST("/packaging-types", anyCompany(deps, service.PermPackagingEdit, deps.Master.CreatePackagingType)...)

	api.GET("/companies", anyCompany(deps, "MASTER.COMPANY.VIEW", deps.Master.ListCompanies)...)
	api.POST("/companies", anyCompany(deps, service.PermCompanyEdit, deps.Master.CreateCompany)...)
}

// registerMaintenance completes the maintenance surface the master data
// screens need. Everything here deactivates rather than deletes (§B1).
func registerMaintenance(api *gin.RouterGroup, deps Dependencies) {
	api.PUT("/movement-types/:id", anyCompany(deps, service.PermMovementTypeEdit, deps.Master.UpdateMovementType)...)
	api.PUT("/uoms/:id", anyCompany(deps, service.PermUOMEdit, deps.Master.UpdateUOM)...)
	api.PUT("/packaging-types/:id", anyCompany(deps, service.PermPackagingEdit, deps.Master.UpdatePackagingType)...)

	companies := api.Group("/companies/:companyId")
	companies.DELETE("", company(deps, service.PermCompanyEdit, deps.Master.DeleteCompany)...)
	// Tanks and condition silos are warehouses of a particular type, not
	// separate entities: same table, same rules, same optimistic lock. These
	// routes are the type-filtered view the maintenance screens open on, so a
	// client never has to know how to spell the filter.
	companies.GET("/tanks", company(deps, "MASTER.WAREHOUSE.VIEW", deps.Master.ListTanks)...)
	companies.GET("/condition-silos", company(deps, "MASTER.WAREHOUSE.VIEW", deps.Master.ListConditionSilos)...)
	companies.PUT("/warehouses/:id/materials", company(deps, service.PermWarehouseEdit, deps.Master.SetWarehouseMaterials)...)
	companies.DELETE("/production-lines/:id", company(deps, service.PermLineEdit, deps.Master.DeleteProductionLine)...)
	companies.DELETE("/seasons/:id", company(deps, service.PermSeasonEdit, deps.Master.DeleteSeason)...)
}

// Company-dependent master data carries the company in the path, which is what
// CompanyAuth reads.
func registerCompanyMaster(api *gin.RouterGroup, deps Dependencies) {
	companies := api.Group("/companies/:companyId")

	companies.GET("", company(deps, "MASTER.COMPANY.VIEW", deps.Master.GetCompany)...)
	companies.PUT("", company(deps, service.PermCompanyEdit, deps.Master.UpdateCompany)...)

	companies.GET("/materials", company(deps, "MASTER.MATERIAL.VIEW", deps.Master.ListCompanyMaterials)...)
	companies.PUT("/materials", company(deps, service.PermMaterialEdit, deps.Master.UpsertCompanyMaterial)...)

	companies.GET("/warehouses", company(deps, "MASTER.WAREHOUSE.VIEW", deps.Master.ListWarehouses)...)
	companies.GET("/warehouses/:id", company(deps, "MASTER.WAREHOUSE.VIEW", deps.Master.GetWarehouse)...)
	companies.POST("/warehouses", company(deps, service.PermWarehouseEdit, deps.Master.CreateWarehouse)...)
	companies.PUT("/warehouses/:id", company(deps, service.PermWarehouseEdit, deps.Master.UpdateWarehouse)...)
	companies.DELETE("/warehouses/:id", company(deps, service.PermWarehouseEdit, deps.Master.DeleteWarehouse)...)

	companies.GET("/production-lines", company(deps, "MASTER.PRODUCTIONLINE.VIEW", deps.Master.ListProductionLines)...)
	companies.POST("/production-lines", company(deps, service.PermLineEdit, deps.Master.CreateProductionLine)...)
	companies.PUT("/production-lines/:id", company(deps, service.PermLineEdit, deps.Master.UpdateProductionLine)...)

	companies.GET("/seasons", company(deps, "MASTER.SEASON.VIEW", deps.Master.ListSeasons)...)
	companies.POST("/seasons", company(deps, service.PermSeasonEdit, deps.Master.CreateSeason)...)
	companies.PUT("/seasons/:id", company(deps, service.PermSeasonEdit, deps.Master.UpdateSeason)...)

	companies.GET("/number-ranges", company(deps, "ADMIN.NUMBERRANGE.MANAGE", deps.Admin.ListNumberRanges)...)
	companies.GET("/audit-log", company(deps, service.PermAuditView, deps.Admin.AuditLog)...)
}

// --- planning (§E3) ------------------------------------------------------

func registerPlanning(api *gin.RouterGroup, deps Dependencies) {
	seasons := api.Group("/companies/:companyId/seasons/:seasonId")
	seasons.GET("/planning-versions", company(deps, service.PermVersionView, deps.Planning.ListVersions)...)
	seasons.POST("/planning-versions", company(deps, service.PermVersionCreate, deps.Planning.CreateVersion)...)

	// These routes take the company from ?companyId or X-Company-Id, so the
	// same handler serves a client that holds only the version id.
	versions := api.Group("/planning-versions")
	versions.GET("/:id", company(deps, service.PermVersionView, deps.Planning.GetVersion)...)
	versions.PUT("/:id", company(deps, service.PermVersionEdit, deps.Planning.UpdateVersion)...)
	versions.POST("/:id/copy", company(deps, service.PermVersionCopy, deps.Planning.Copy)...)
	versions.POST("/:id/submit", company(deps, service.PermVersionSubmit, deps.Planning.Submit())...)
	versions.POST("/:id/approve", company(deps, service.PermVersionApprove, deps.Planning.Approve())...)
	versions.POST("/:id/lock", company(deps, service.PermVersionLock, deps.Planning.Lock())...)
	versions.POST("/:id/cancel", company(deps, service.PermVersionCancel, deps.Planning.Cancel())...)

	plans := api.Group("/plans")
	// The matrix routes are registered before the parameterised ones so that
	// "matrix" is never mistaken for a header id.
	plans.GET("/matrix", company(deps, service.PermPlanItemView, deps.Planning.GetMatrix)...)
	// The set route returns every product the version plans over the window;
	// the single route above still serves one product at a time.
	plans.GET("/matrix/all", company(deps, service.PermPlanItemView, deps.Planning.GetMatrixSet)...)
	plans.POST("/matrix", company(deps, service.PermPlanItemEdit, deps.Planning.SaveMatrix)...)
	plans.GET("", company(deps, service.PermPlanItemView, deps.Planning.ListPlans)...)
	plans.GET("/:headerId", company(deps, service.PermPlanItemView, deps.Planning.GetPlan)...)
	plans.GET("/:headerId/items", company(deps, service.PermPlanItemView, deps.Planning.PlanItems)...)
}

// --- actual (§E4) --------------------------------------------------------

func registerActual(api *gin.RouterGroup, deps Dependencies) {
	actuals := api.Group("/actuals")
	actuals.GET("", company(deps, service.PermActualView, deps.Actual.List)...)
	actuals.POST("", company(deps, service.PermActualCreate, deps.Actual.Create)...)
	actuals.GET("/:headerId", company(deps, service.PermActualView, deps.Actual.Get)...)
	actuals.PUT("/:headerId", company(deps, service.PermActualEdit, deps.Actual.Update)...)
	actuals.POST("/:headerId/post", company(deps, service.PermActualPost, deps.Actual.Post)...)
	actuals.POST("/:headerId/reverse", company(deps, service.PermActualReverse, deps.Actual.Reverse)...)
}

// --- inventory (§E5) -----------------------------------------------------

func registerInventory(api *gin.RouterGroup, deps Dependencies) {
	inventory := api.Group("/inventory")
	inventory.GET("/movements", company(deps, service.PermMovementView, deps.Inventory.ListMovements)...)
	inventory.POST("/movements", company(deps, service.PermMovementCreate, deps.Inventory.CreateMovement)...)
	inventory.POST("/movements/:id/reverse", company(deps, service.PermMovementReverse, deps.Inventory.ReverseMovement)...)
	inventory.POST("/transfers", company(deps, service.PermTransferCreate, deps.Inventory.Transfer)...)
	inventory.POST("/adjustments", company(deps, service.PermAdjustCreate, deps.Inventory.CreateMovement)...)
	inventory.GET("/balances", company(deps, service.PermBalanceView, deps.Inventory.Balances)...)
	inventory.GET("/capacity", company(deps, service.PermBalanceView, deps.Inventory.Capacity)...)
	inventory.POST("/reconcile", company(deps, service.PermMovementCreate, deps.Inventory.Reconcile)...)
}

// --- cane supply ---------------------------------------------------------

// Cane varieties are cross-company master data like materials; growers and
// fields belong to one company and carry it in the path.
func registerCane(api *gin.RouterGroup, deps Dependencies) {
	varieties := api.Group("/cane-varieties")
	varieties.GET("", anyCompany(deps, service.PermCaneVarietyView, deps.Cane.ListVarieties)...)
	varieties.GET("/:id", anyCompany(deps, service.PermCaneVarietyView, deps.Cane.GetVariety)...)
	varieties.POST("", anyCompany(deps, service.PermCaneVarietyEdit, deps.Cane.CreateVariety)...)
	varieties.PUT("/:id", anyCompany(deps, service.PermCaneVarietyEdit, deps.Cane.UpdateVariety)...)
	varieties.DELETE("/:id", anyCompany(deps, service.PermCaneVarietyEdit, deps.Cane.DeleteVariety)...)

	companies := api.Group("/companies/:companyId")

	companies.GET("/growers", company(deps, service.PermGrowerView, deps.Cane.ListGrowers)...)
	companies.GET("/growers/:id", company(deps, service.PermGrowerView, deps.Cane.GetGrower)...)
	companies.POST("/growers", company(deps, service.PermGrowerEdit, deps.Cane.CreateGrower)...)
	companies.PUT("/growers/:id", company(deps, service.PermGrowerEdit, deps.Cane.UpdateGrower)...)
	companies.DELETE("/growers/:id", company(deps, service.PermGrowerEdit, deps.Cane.DeleteGrower)...)

	companies.GET("/cane-fields", company(deps, service.PermCaneFieldView, deps.Cane.ListFields)...)
	companies.GET("/cane-fields/:id", company(deps, service.PermCaneFieldView, deps.Cane.GetField)...)
	companies.POST("/cane-fields", company(deps, service.PermCaneFieldEdit, deps.Cane.CreateField)...)
	companies.PUT("/cane-fields/:id", company(deps, service.PermCaneFieldEdit, deps.Cane.UpdateField)...)
	companies.DELETE("/cane-fields/:id", company(deps, service.PermCaneFieldEdit, deps.Cane.DeleteField)...)

	// The matrix routes come before the parameterised ones so that "matrix" is
	// never mistaken for a document id.
	harvest := api.Group("/harvest-plans")
	harvest.GET("/matrix", company(deps, service.PermHarvestPlanView, deps.Cane.GetHarvestMatrix)...)
	harvest.POST("/matrix", company(deps, service.PermHarvestPlanEdit, deps.Cane.SaveHarvestMatrix)...)
	harvest.GET("", company(deps, service.PermHarvestPlanView, deps.Cane.ListHarvestPlans)...)
	harvest.GET("/:headerId/items", company(deps, service.PermHarvestPlanView, deps.Cane.HarvestPlanItems)...)

	deliveries := api.Group("/cane-deliveries")
	deliveries.GET("", company(deps, service.PermDeliveryView, deps.Cane.ListDeliveries)...)
	deliveries.POST("", company(deps, service.PermDeliveryCreate, deps.Cane.CreateDelivery)...)
	deliveries.GET("/:id", company(deps, service.PermDeliveryView, deps.Cane.GetDelivery)...)
	deliveries.PUT("/:id", company(deps, service.PermDeliveryEdit, deps.Cane.UpdateDelivery)...)
	deliveries.POST("/:id/post", company(deps, service.PermDeliveryPost, deps.Cane.PostDelivery)...)
	deliveries.POST("/:id/reverse", company(deps, service.PermDeliveryReverse, deps.Cane.ReverseDelivery)...)
}

// --- reporting (§E6) -----------------------------------------------------

func registerReports(api *gin.RouterGroup, deps Dependencies) {
	reports := api.Group("/reports")

	// The consolidated route validates each company id itself, so it is not
	// wrapped in the single-company middleware.
	reports.GET("/plan-vs-actual/consolidated", deps.Reports.PlanVsActualConsolidated)

	reports.GET("/plan-vs-actual", company(deps, service.PermReportPlanActual, deps.Reports.PlanVsActual)...)
	reports.GET("/production-summary", company(deps, service.PermReportProduction, deps.Reports.ProductionSummary)...)
	reports.GET("/inventory-movement", company(deps, service.PermReportInventory, deps.Reports.InventoryMovements)...)
	reports.GET("/capacity-utilisation", company(deps, service.PermReportCapacity, deps.Reports.CapacityUtilisation)...)
	reports.GET("/cane-plan-vs-actual", company(deps, service.PermReportCane, deps.Cane.CanePlanVsActual)...)
}

// --- data dictionary & browser (§E7) -------------------------------------

func registerDataDictionary(api *gin.RouterGroup, deps Dependencies) {
	dd := api.Group("/dd")
	dd.GET("/tables", anyCompany(deps, service.PermDDView, deps.Dict.ListTables)...)
	dd.GET("/tables/:tableName", anyCompany(deps, service.PermDDView, deps.Dict.GetTable)...)
	dd.GET("/tables/:tableName/fields", anyCompany(deps, service.PermDDView, deps.Dict.Fields)...)
	dd.PUT("/tables/:tableName", anyCompany(deps, service.PermDDMaintain, deps.Dict.UpdateTable)...)
	dd.PUT("/fields/:id", anyCompany(deps, service.PermDDMaintain, deps.Dict.UpdateField)...)
	dd.GET("/domains", anyCompany(deps, service.PermDDView, deps.Dict.ListDomains)...)

	browser := api.Group("/browser")
	browser.GET("/:tableName", anyCompany(deps, service.PermBrowserView, deps.Dict.Browse)...)
	browser.GET("/:tableName/export", anyCompany(deps, service.PermBrowserExport, deps.Dict.Export)...)
}

// --- administration ------------------------------------------------------

func registerAdmin(api *gin.RouterGroup, deps Dependencies) {
	admin := api.Group("/admin")
	admin.GET("/users", anyCompany(deps, service.PermUserManage, deps.Admin.ListUsers)...)
	admin.POST("/users", anyCompany(deps, service.PermUserManage, deps.Admin.CreateUser)...)
	admin.GET("/users/:id", anyCompany(deps, service.PermUserManage, deps.Admin.GetUser)...)
	admin.PUT("/users/:id", anyCompany(deps, service.PermUserManage, deps.Admin.UpdateUser)...)
	admin.GET("/users/:id/assignments", anyCompany(deps, service.PermUserManage, deps.Admin.UserAssignments)...)
	admin.POST("/users/:id/reset-password", anyCompany(deps, service.PermUserManage, deps.Admin.ResetPassword)...)
	admin.GET("/roles", anyCompany(deps, service.PermRoleManage, deps.Admin.ListRoles)...)
	admin.GET("/permissions", anyCompany(deps, service.PermRoleManage, deps.Admin.ListPermissions)...)
}
