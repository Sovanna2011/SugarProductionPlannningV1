sap.ui.define([
	"sugar/planning/controller/BaseController"
], function (BaseController) {
	"use strict";

	return BaseController.extend("sugar.planning.controller.DataDictionary", {

		permissions: [
			{ name: "Maintain", permission: "DD.MAINTAIN" }
		],

		onInit: function () {
			this.initViewModel({ tables: [], allTables: [], fields: [], selectedTable: {} });
			this.getRouter().getRoute("dictionary").attachPatternMatched(this._onDisplay, this);
		},

		_onDisplay: function () {
			if (!this.api.isLoggedIn()) {
				this.getRouter().navTo("login", {}, true);
				return;
			}
			this.applyDefaultCompany();
			this._loadTables();
		},

		_loadTables: function () {
			var model = this.getViewModel();
			var query = this.api.buildQuery({ companyId: this.getCompanyId() });

			this.api.get("/dd/tables" + query).then(function (tables) {
				model.setProperty("/allTables", tables || []);
				model.setProperty("/tables", tables || []);
			}).catch(function () { /* reported */ });
		},

		onSearch: function (event) {
			var term = (event.getParameter("newValue") || "").toLowerCase();
			var model = this.getViewModel();
			var all = model.getProperty("/allTables") || [];

			model.setProperty("/tables", term ? all.filter(function (table) {
				return table.tableName.toLowerCase().indexOf(term) >= 0 ||
					(table.descriptionEn || "").toLowerCase().indexOf(term) >= 0;
			}) : all);
		},

		onTableSelect: function (event) {
			var table = event.getParameter("listItem").getBindingContext("view").getObject();
			var model = this.getViewModel();
			var query = this.api.buildQuery({ companyId: this.getCompanyId() });

			model.setProperty("/selectedTable", table);
			this.api.get("/dd/tables/" + table.tableName + "/fields" + query)
				.then(function (fields) {
					model.setProperty("/fields", fields || []);
				}).catch(function () { /* reported */ });
		}
	});
});
