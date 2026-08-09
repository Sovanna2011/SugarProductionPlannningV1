sap.ui.define([
	"sugar/planning/controller/BaseController",
	"sap/ui/table/Column",
	"sap/m/Input",
	"sap/m/Label",
	"sap/m/MessageBox"
], function (BaseController, Column, Input, Label, MessageBox) {
	"use strict";

	/**
	 * CaneSupply is the agricultural half of the plan: where the cane comes
	 * from, on which day, and what actually crossed the weighbridge.
	 *
	 * The harvest grid is a Date × Grower matrix built the same way the
	 * production matrix is — columns generated at runtime from the company's
	 * growers — and it lives inside the same planning version, so approving a
	 * version freezes the cane plan and the production plan together.
	 */
	return BaseController.extend("sugar.planning.controller.CaneSupply", {

		permissions: [
			{ name: "EditPlan", permission: "CANE.PLAN.EDIT" },
			{ name: "ViewPlan", permission: "CANE.PLAN.VIEW" },
			{ name: "CreateDelivery", permission: "CANE.DELIVERY.CREATE" },
			{ name: "PostDelivery", permission: "CANE.DELIVERY.POST" },
			{ name: "ReverseDelivery", permission: "CANE.DELIVERY.REVERSE" }
		],

		onInit: function () {
			this.initViewModel({
				seasons: [], versions: [], growers: [], fields: [], varieties: [],
				warehouses: [], harvestRows: [], deliveries: [], report: [],
				versionStatus: "", planLockedReason: "", harvestTotal: "0",
				groupBy: "grower", supplyType: "",
				dateFrom: this._day(-2), dateTo: this._day(4),
				canEditPlan: false
			});

			this.getRouter().getRoute("cane").attachPatternMatched(this._onDisplay, this);
		},

		_onDisplay: function () {
			if (!this.api.isLoggedIn()) {
				this.getRouter().navTo("login", {}, true);
				return;
			}
			this.applyDefaultCompany();
			this._loadCompanyContext();
		},

		onCompanySelected: function () {
			var model = this.getViewModel();
			model.setProperty("/seasonId", null);
			model.setProperty("/versionId", null);
			model.setProperty("/harvestRows", []);
			model.setProperty("/deliveries", []);
			model.setProperty("/report", []);
			this._loadCompanyContext();
		},

		_loadCompanyContext: function () {
			var companyId = this.getCompanyId();
			if (!companyId) {
				return;
			}
			var model = this.getViewModel();
			var that = this;

			Promise.all([
				this.api.get("/companies/" + companyId + "/seasons?size=100"),
				this.api.get("/companies/" + companyId + "/growers?size=200"),
				this.api.get("/companies/" + companyId + "/cane-fields?size=500"),
				this.api.get("/cane-varieties?size=100&companyId=" + companyId),
				this.api.get("/companies/" + companyId + "/warehouses?size=100")
			]).then(function (results) {
				model.setProperty("/seasons", results[0] || []);
				model.setProperty("/growers", results[1] || []);
				model.setProperty("/fields", results[2] || []);
				model.setProperty("/varieties", results[3] || []);
				model.setProperty("/warehouses", results[4] || []);

				var open = (results[0] || []).filter(function (season) {
					return season.status === "OPEN";
				})[0];
				if (open) {
					model.setProperty("/seasonId", String(open.id));
					that.onSeasonChange();
				}
				that._loadDeliveries();
			}).catch(function () { /* ApiClient already reported it */ });
		},

		onSeasonChange: function () {
			var companyId = this.getCompanyId();
			var seasonId = this.getViewModel().getProperty("/seasonId");
			var model = this.getViewModel();
			var that = this;

			if (!companyId || !seasonId) {
				return;
			}
			this.api.get("/companies/" + companyId + "/seasons/" + seasonId + "/planning-versions")
				.then(function (versions) {
					model.setProperty("/versions", versions || []);
					var draft = (versions || []).filter(function (entry) {
						return entry.status === "DRAFT";
					})[0] || (versions || [])[0];
					if (draft) {
						model.setProperty("/versionId", String(draft.id));
						that.onVersionChange();
					}
				}).catch(function () { /* reported */ });
		},

		onVersionChange: function () {
			var model = this.getViewModel();
			var versionId = Number(model.getProperty("/versionId"));
			var version = (model.getProperty("/versions") || []).filter(function (entry) {
				return entry.id === versionId;
			})[0];

			model.setProperty("/versionStatus", version ? version.status : "");
			this._evaluateEditability(version);
			this._loadHarvest();
		},

		onRangeChange: function () {
			this._loadHarvest();
			this._loadDeliveries();
			this._loadReport();
		},

		onTabSelect: function (event) {
			var key = event.getParameter("key");
			if (key === "deliveries") {
				this._loadDeliveries();
			} else if (key === "report") {
				this._loadReport();
			}
		},

		/**
		 * _evaluateEditability keeps the UI honest about two separate things:
		 * whether this user may plan cane in this company, and whether the
		 * version's status still allows any change. The server enforces both
		 * regardless — and so does a database trigger.
		 */
		_evaluateEditability: function (version) {
			var model = this.getViewModel();
			var mayEdit = this.api.hasPermission(this.getCompanyId(), "CANE.PLAN.EDIT");
			var status = version ? version.status : null;
			var statusAllows = status === "DRAFT" || status === "SUBMITTED";

			model.setProperty("/canEditPlan", mayEdit && statusAllows);

			var reason = "";
			if (!mayEdit) {
				reason = this.i18n("noEditPermission");
			} else if (status && !statusAllows) {
				reason = this.i18n("versionNotEditable", [status]);
			}
			model.setProperty("/planLockedReason", reason);
		},

		// --- the harvest grid ------------------------------------------------

		_loadHarvest: function () {
			var model = this.getViewModel();
			var companyId = this.getCompanyId();
			var versionId = model.getProperty("/versionId");
			var that = this;

			if (!companyId || !versionId) {
				return;
			}
			var query = this.api.buildQuery({
				companyId: companyId,
				versionId: versionId,
				dateFrom: model.getProperty("/dateFrom"),
				dateTo: model.getProperty("/dateTo")
			});

			this.api.get("/harvest-plans/matrix" + query).then(function (matrix) {
				that._renderHarvest(matrix);
			}).catch(function () { /* reported */ });
		},

		/**
		 * _renderHarvest rebuilds the columns from the growers the server
		 * returned, then flattens the row/value payload into one object per
		 * date carrying a property per grower.
		 */
		_renderHarvest: function (matrix) {
			var model = this.getViewModel();
			var growers = matrix.growers || [];

			model.setProperty("/versionStatus", matrix.versionStatus);

			var rows = (matrix.rows || []).map(function (row) {
				var flat = { planDate: row.planDate };
				growers.forEach(function (grower) {
					flat["g_" + grower.growerId] = "";
				});
				(row.values || []).forEach(function (value) {
					flat["g_" + value.growerId] = value.plannedTons;
				});
				return flat;
			});

			model.setProperty("/harvestRows", rows);
			this._rebuildHarvestColumns(growers);
			this._recalculate();
		},

		_rebuildHarvestColumns: function (growers) {
			var table = this.byId("harvestTable");
			var editable = "{= ${view>/canEditPlan} }";

			// Keep the first (date) column and replace everything after it with
			// one column per grower.
			table.getColumns().slice(1).forEach(function (column) {
				table.removeColumn(column);
				column.destroy();
			});

			growers.forEach(function (grower) {
				// Purchased cane is marked in the heading: it is the cane that
				// carries a contract and a bill.
				var heading = grower.growerCode +
					(grower.supplyType === "PURCHASED" ? " ⁺" : "");

				table.addColumn(new Column({
					width: "9rem",
					hAlign: "End",
					label: new Label({ text: heading, tooltip: grower.growerName }),
					template: new Input({
						value: "{view>g_" + grower.growerId + "}",
						type: "Number",
						textAlign: "End",
						editable: editable,
						change: this.onCellChange.bind(this)
					})
				}));
			}, this);

			this._growers = growers;
		},

		onCellChange: function () {
			this._recalculate();
		},

		_recalculate: function () {
			var model = this.getViewModel();
			var rows = model.getProperty("/harvestRows") || [];
			var growers = this._growers || [];
			var total = 0;

			rows.forEach(function (row) {
				growers.forEach(function (grower) {
					total += Number(row["g_" + grower.growerId]) || 0;
				});
			});
			model.setProperty("/harvestTotal", String(Math.round(total * 1000) / 1000));
		},

		/**
		 * onSaveHarvest sends the whole grid. An empty cell is omitted, which
		 * the server reads as a deletion — that is the point of the full save.
		 */
		onSaveHarvest: function () {
			var model = this.getViewModel();
			var growers = this._growers || [];
			var that = this;

			var rows = (model.getProperty("/harvestRows") || []).map(function (row) {
				var values = [];
				growers.forEach(function (grower) {
					var raw = row["g_" + grower.growerId];
					if (raw === "" || raw === null || raw === undefined) {
						return;
					}
					values.push({ growerId: grower.growerId, plannedTons: String(raw) });
				});
				return { planDate: row.planDate, values: values };
			});

			this.api.post("/harvest-plans/matrix?companyId=" + this.getCompanyId(), {
				versionId: Number(model.getProperty("/versionId")),
				dateFrom: model.getProperty("/dateFrom"),
				dateTo: model.getProperty("/dateTo"),
				rows: rows,
				partialUpdate: false
			}).then(function (matrix) {
				that._renderHarvest(matrix);
				that.toast("saved");
			}).catch(function () { /* reported */ });
		},

		// --- weighbridge tickets ---------------------------------------------

		_loadDeliveries: function () {
			var model = this.getViewModel();
			var companyId = this.getCompanyId();
			var that = this;

			if (!companyId) {
				return;
			}
			var query = this.api.buildQuery({
				companyId: companyId,
				dateFrom: model.getProperty("/dateFrom"),
				dateTo: model.getProperty("/dateTo"),
				size: 100
			});

			this.api.get("/cane-deliveries" + query).then(function (deliveries) {
				var growers = model.getProperty("/growers") || [];
				model.setProperty("/deliveries", (deliveries || []).map(function (delivery) {
					var grower = growers.filter(function (entry) {
						return entry.id === delivery.growerId;
					})[0];
					delivery.growerLabel = grower ? grower.growerCode + " — " + grower.growerName : "";
					return delivery;
				}));
			}).catch(function () { /* reported */ });
		},

		onNewDelivery: function () {
			var model = this.getViewModel();
			var growers = model.getProperty("/growers") || [];
			var yards = (model.getProperty("/warehouses") || []).filter(function (warehouse) {
				return warehouse.warehouseType === "PRODUCTION_STORAGE";
			});
			var that = this;

			if (!growers.length) {
				this.toast("caneNoGrowers");
				return;
			}

			// A short prompt is enough here: material, movement type and unit
			// are all filled in by the server, and the price comes from the
			// grower's contract.
			MessageBox.show(this.i18n("caneTicketPrompt"), {
				icon: MessageBox.Icon.QUESTION,
				title: this.i18n("caneNewTicket"),
				actions: [MessageBox.Action.OK, MessageBox.Action.CANCEL],
				onClose: function (action) {
					if (action !== MessageBox.Action.OK) {
						return;
					}
					that.api.post("/cane-deliveries?companyId=" + that.getCompanyId(), {
						deliveryDate: model.getProperty("/dateTo"),
						growerId: growers[0].id,
						warehouseId: yards.length ? yards[0].id : null,
						grossTons: "40",
						tareTons: "12"
					}).then(function () {
						that._loadDeliveries();
						that.toast("saved");
					}).catch(function () { /* reported */ });
				}
			});
		},

		onPostDelivery: function (event) {
			var delivery = event.getSource().getBindingContext("view").getObject();
			var that = this;

			this.api.post("/cane-deliveries/" + delivery.id + "/post?companyId=" + this.getCompanyId(), {})
				.then(function () {
					that._loadDeliveries();
					that.toast("posted");
				}).catch(function () { /* reported */ });
		},

		onReverseDelivery: function (event) {
			var delivery = event.getSource().getBindingContext("view").getObject();
			var that = this;

			this.api.post("/cane-deliveries/" + delivery.id + "/reverse?companyId=" + this.getCompanyId(),
				{ remark: this.i18n("caneReversalRemark") })
				.then(function () {
					that._loadDeliveries();
					that.toast("reversed");
				}).catch(function () { /* reported */ });
		},

		// --- the cane report --------------------------------------------------

		onReportChange: function () {
			this._loadReport();
		},

		_loadReport: function () {
			var model = this.getViewModel();
			var companyId = this.getCompanyId();

			if (!companyId) {
				return;
			}
			var query = this.api.buildQuery({
				companyId: companyId,
				versionId: model.getProperty("/versionId"),
				groupBy: model.getProperty("/groupBy"),
				supplyType: model.getProperty("/supplyType"),
				dateFrom: model.getProperty("/dateFrom"),
				dateTo: model.getProperty("/dateTo")
			});

			this.api.get("/reports/cane-plan-vs-actual" + query).then(function (rows) {
				model.setProperty("/report", rows || []);
			}).catch(function () { /* reported */ });
		},

		_day: function (offset) {
			var date = new Date();
			date.setDate(date.getDate() + offset);
			return date.toISOString().slice(0, 10);
		}
	});
});
