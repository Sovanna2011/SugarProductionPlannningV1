sap.ui.define([
	"sugar/planning/controller/BaseController",
	"sap/ui/table/Column",
	"sap/m/Text",
	"sap/m/Label"
], function (BaseController, Column, Text, Label) {
	"use strict";

	return BaseController.extend("sugar.planning.controller.DataBrowser", {

		permissions: [
			{ name: "Browse", permission: "BROWSER.VIEW" },
			{ name: "Export", permission: "BROWSER.EXPORT" }
		],

		onInit: function () {
			this.initViewModel({
				tables: [], fields: [], rows: [], total: 0, module: "", tableName: null
			});
			this.getRouter().getRoute("browser").attachPatternMatched(this._onDisplay, this);
		},

		_onDisplay: function () {
			if (!this.api.isLoggedIn()) {
				this.getRouter().navTo("login", {}, true);
				return;
			}
			this.applyDefaultCompany();
			this.onModuleChange();
		},

		onCompanySelected: function () {
			this.onModuleChange();
		},

		/** Only browsable tables are offered — the whitelist comes from the server. */
		onModuleChange: function () {
			var model = this.getViewModel();
			var query = this.api.buildQuery({
				module: model.getProperty("/module"),
				browsableOnly: "true",
				companyId: this.getCompanyId()
			});

			this.api.get("/dd/tables" + query).then(function (tables) {
				model.setProperty("/tables", tables || []);
			}).catch(function () { /* reported */ });
		},

		/** The column labels come from the dictionary, not from the raw column names. */
		onTableChange: function () {
			var model = this.getViewModel();
			var tableName = model.getProperty("/tableName");
			if (!tableName) {
				return;
			}
			var query = this.api.buildQuery({ companyId: this.getCompanyId() });

			this.api.get("/dd/tables/" + tableName + "/fields" + query).then(function (fields) {
				model.setProperty("/fields", fields || []);
			}).catch(function () { /* reported */ });
		},

		onRun: function () {
			var model = this.getViewModel();
			var tableName = model.getProperty("/tableName");
			var that = this;

			if (!tableName) {
				this.toast("selectTableFirst");
				return;
			}

			var query = this.api.buildQuery({
				companyId: this.getCompanyId(),
				size: 200
			});

			this.api.get("/browser/" + tableName + query, { withMeta: true })
				.then(function (payload) {
					var result = payload.data;
					model.setProperty("/rows", result.rows || []);
					model.setProperty("/total", payload.meta ? payload.meta.total : 0);
					that._rebuildColumns(result.fields || []);
				}).catch(function () { /* reported */ });
		},

		_rebuildColumns: function (fieldNames) {
			var table = this.byId("resultTable");
			var labels = {};

			(this.getViewModel().getProperty("/fields") || []).forEach(function (field) {
				labels[field.fieldName] = field.labelMedium || field.fieldName;
			});

			table.removeAllColumns();
			fieldNames.forEach(function (name) {
				table.addColumn(new Column({
					width: "11rem",
					label: new Label({ text: labels[name] || name, tooltip: name }),
					template: new Text({ text: "{view>" + name + "}", wrapping: false })
				}));
			});
		}
	});
});
