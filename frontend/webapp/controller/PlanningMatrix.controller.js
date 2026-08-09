sap.ui.define([
	"sugar/planning/controller/BaseController",
	"sap/ui/table/Column",
	"sap/m/Input",
	"sap/m/Label"
], function (BaseController, Column, Input, Label) {
	"use strict";

	return BaseController.extend("sugar.planning.controller.PlanningMatrix", {

		permissions: [
			{ name: "Edit", permission: "PLAN.ITEM.EDIT" },
			{ name: "View", permission: "PLAN.ITEM.VIEW" }
		],

		onInit: function () {
			this.initViewModel({
				seasons: [], versions: [], materials: [], processes: [],
				movementTypes: [], lines: [], rows: [],
				loaded: false, canEdit: false, grandTotal: "0",
				dateFrom: this._today(0), dateTo: this._today(6),
				versionStatus: "", readOnlyReason: ""
			});

			this.getRouter().getRoute("planning").attachPatternMatched(this._onDisplay, this);
		},

		_onDisplay: function () {
			if (!this.api.isLoggedIn()) {
				this.getRouter().navTo("login", {}, true);
				return;
			}
			this.applyDefaultCompany();
			this._loadCompanyContext();
		},

		/**
		 * onCompanySelected reloads everything that is company-dependent.
		 * Switching the drop-down genuinely switches the working context — the
		 * same user may be a planner in one company and a display user in the
		 * next, so the permissions are re-evaluated too.
		 */
		onCompanySelected: function () {
			var model = this.getViewModel();
			model.setProperty("/seasonId", null);
			model.setProperty("/versionId", null);
			model.setProperty("/loaded", false);
			model.setProperty("/rows", []);
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
				this.api.get("/companies/" + companyId + "/production-lines?size=100"),
				this.api.get("/companies/" + companyId + "/materials?size=200"),
				this.api.get("/movement-types?size=100&companyId=" + companyId),
				this.api.get("/processes?size=100&companyId=" + companyId)
			]).then(function (results) {
				model.setProperty("/seasons", results[0] || []);
				model.setProperty("/lines", results[1] || []);

				// The value help lists only materials this company actually
				// plans — the company relevance flags of §14.
				model.setProperty("/materials", (results[2] || [])
					.filter(function (entry) { return entry.planningEnabled; })
					.map(function (entry) { return entry.material; })
					.filter(Boolean));

				model.setProperty("/movementTypes", (results[3] || [])
					.filter(function (entry) { return entry.isPlanningRelevant; }));
				model.setProperty("/processes", results[4] || []);

				var open = (results[0] || []).filter(function (season) {
					return season.status === "OPEN";
				})[0];
				if (open) {
					model.setProperty("/seasonId", String(open.id));
					that.onSeasonChange();
				}
			}).catch(function () { /* ApiClient already reported it */ });
		},

		onSeasonChange: function () {
			var companyId = this.getCompanyId();
			var seasonId = this.getViewModel().getProperty("/seasonId");
			var model = this.getViewModel();

			if (!companyId || !seasonId) {
				return;
			}
			this.api.get("/companies/" + companyId + "/seasons/" + seasonId + "/planning-versions")
				.then(function (versions) {
					model.setProperty("/versions", versions || []);
					model.setProperty("/versionId", null);
					model.setProperty("/loaded", false);
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
			model.setProperty("/loaded", false);
		},

		onMaterialChange: function () {
			this.getViewModel().setProperty("/loaded", false);
		},

		onSelectionChange: function () {
			this.getViewModel().setProperty("/loaded", false);
		},

		/**
		 * _evaluateEditability combines two independent things: whether the
		 * user may edit plan data in this company at all, and whether the
		 * version's status still allows a change (§F3). The server enforces
		 * both regardless — this only keeps the UI honest.
		 */
		_evaluateEditability: function (version) {
			var model = this.getViewModel();
			var mayEdit = this.api.hasPermission(this.getCompanyId(), "PLAN.ITEM.EDIT");
			var status = version ? version.status : null;
			var statusAllows = status === "DRAFT" || status === "SUBMITTED";

			model.setProperty("/canEdit", mayEdit && statusAllows);

			var reason = "";
			if (!mayEdit) {
				reason = this.i18n("noEditPermission");
			} else if (!statusAllows && status) {
				reason = this.i18n("versionNotEditable", [status]);
			}
			model.setProperty("/readOnlyReason", reason);
		},

		// --- loading the grid ------------------------------------------------

		onLoad: function () {
			if (!this.requireCompany()) {
				return;
			}
			var model = this.getViewModel();
			var that = this;

			var query = this.api.buildQuery({
				companyId: this.getCompanyId(),
				versionId: model.getProperty("/versionId"),
				movementTypeId: model.getProperty("/movementTypeId"),
				materialId: model.getProperty("/materialId"),
				processId: model.getProperty("/processId"),
				dateFrom: model.getProperty("/dateFrom"),
				dateTo: model.getProperty("/dateTo")
			});

			if (!model.getProperty("/versionId") || !model.getProperty("/movementTypeId") ||
				!model.getProperty("/materialId")) {
				this.toast("matrixSelectionIncomplete");
				return;
			}

			this.api.get("/plans/matrix" + query).then(function (matrix) {
				that._renderMatrix(matrix);
			}).catch(function () { /* reported */ });
		},

		/**
		 * _renderMatrix rebuilds the table columns from the production lines the
		 * server returned, then flattens the row/value payload into one object
		 * per date with a property per line.
		 */
		_renderMatrix: function (matrix) {
			var model = this.getViewModel();
			var lines = matrix.lines || [];

			model.setProperty("/lines", lines);
			model.setProperty("/versionStatus", matrix.versionStatus);

			var rows = (matrix.rows || []).map(function (row) {
				var flat = { planDate: row.planDate, rowTotal: "0" };
				var total = 0;

				lines.forEach(function (line) {
					flat["line_" + line.productionLineId] = "";
				});
				(row.values || []).forEach(function (value) {
					if (value.productionLineId === null || value.productionLineId === undefined) {
						return;
					}
					flat["line_" + value.productionLineId] = value.quantity;
					total += Number(value.quantity) || 0;
				});

				flat.rowTotal = String(total);
				return flat;
			});

			model.setProperty("/rows", rows);
			this._rebuildColumns(lines);
			this._recalculate();
			model.setProperty("/loaded", true);
		},

		_rebuildColumns: function (lines) {
			var table = this.byId("matrixTable");
			var editable = "{= ${view>/canEdit} }";

			// Keep the first (date) and last (row total) columns, replacing
			// everything in between with one column per line.
			var columns = table.getColumns();
			columns.slice(1, columns.length - 1).forEach(function (column) {
				table.removeColumn(column);
				column.destroy();
			});

			var insertAt = 1;
			lines.forEach(function (line) {
				var path = "view>line_" + line.productionLineId;
				var column = new Column({
					width: "9rem",
					hAlign: "End",
					label: new Label({ text: line.lineCode, tooltip: line.lineName }),
					template: new Input({
						value: "{" + path + "}",
						type: "Number",
						textAlign: "End",
						editable: editable,
						change: this.onCellChange.bind(this)
					})
				});
				table.insertColumn(column, insertAt++);
			}, this);
		},

		onCellChange: function () {
			this._recalculate();
		},

		/** Row and column totals are recomputed locally so typing feels instant. */
		_recalculate: function () {
			var model = this.getViewModel();
			var lines = model.getProperty("/lines") || [];
			var rows = model.getProperty("/rows") || [];
			var grand = 0;

			rows.forEach(function (row) {
				var total = 0;
				lines.forEach(function (line) {
					total += Number(row["line_" + line.productionLineId]) || 0;
				});
				row.rowTotal = String(total);
				grand += total;
			});

			model.setProperty("/rows", rows);
			model.setProperty("/grandTotal", String(grand));
			model.refresh(true);
		},

		// --- saving ----------------------------------------------------------

		/**
		 * onSave sends the grid back in the shape the server accepts. An empty
		 * cell is sent as an omitted value so the server deletes it, which is
		 * what makes the screen round-trippable rather than append-only.
		 */
		onSave: function () {
			if (!this.requireCompany()) {
				return;
			}
			var model = this.getViewModel();
			var lines = model.getProperty("/lines") || [];
			var that = this;

			var rows = (model.getProperty("/rows") || []).map(function (row) {
				var values = [];
				lines.forEach(function (line) {
					var raw = row["line_" + line.productionLineId];
					if (raw === "" || raw === null || raw === undefined) {
						return; // an omitted cell is a deletion
					}
					values.push({
						productionLineId: line.productionLineId,
						quantity: String(raw)
					});
				});
				return { planDate: row.planDate, values: values };
			});

			var material = (model.getProperty("/materials") || []).filter(function (entry) {
				return String(entry.id) === String(model.getProperty("/materialId"));
			})[0];

			this.api.post("/plans/matrix?companyId=" + this.getCompanyId(), {
				seasonId: Number(model.getProperty("/seasonId")),
				versionId: Number(model.getProperty("/versionId")),
				movementTypeId: Number(model.getProperty("/movementTypeId")),
				materialId: Number(model.getProperty("/materialId")),
				processId: model.getProperty("/processId")
					? Number(model.getProperty("/processId")) : null,
				uomId: material ? material.baseUomId : null,
				dateFrom: model.getProperty("/dateFrom"),
				dateTo: model.getProperty("/dateTo"),
				rows: rows,
				partialUpdate: false
			}).then(function (matrix) {
				that._renderMatrix(matrix);
				that.toast("matrixSaved");
			}).catch(function () { /* reported */ });
		},

		_today: function (offsetDays) {
			var date = new Date();
			date.setUTCDate(date.getUTCDate() + offsetDays);
			return date.toISOString().substring(0, 10);
		}
	});
});
