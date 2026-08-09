sap.ui.define([
	"sugar/planning/controller/BaseController"
], function (BaseController) {
	"use strict";

	return BaseController.extend("sugar.planning.controller.Inventory", {

		permissions: [
			{ name: "Transfer", permission: "INV.TRANSFER.CREATE" },
			{ name: "View", permission: "INV.BALANCE.VIEW" }
		],

		onInit: function () {
			this.initViewModel({ balances: [], capacity: [], movements: [] });
			this.getRouter().getRoute("inventory").attachPatternMatched(this._onDisplay, this);
		},

		_onDisplay: function () {
			if (!this.api.isLoggedIn()) {
				this.getRouter().navTo("login", {}, true);
				return;
			}
			this.applyDefaultCompany();
			this.onLoad();
		},

		onCompanySelected: function () {
			this.onLoad();
		},

		onLoad: function () {
			var companyId = this.getCompanyId();
			if (!companyId) {
				return;
			}
			var model = this.getViewModel();

			Promise.all([
				this.api.get("/inventory/balances?companyId=" + companyId),
				this.api.get("/inventory/capacity?companyId=" + companyId),
				this.api.get("/inventory/movements?companyId=" + companyId + "&size=100")
			]).then(function (results) {
				model.setProperty("/balances", results[0] || []);
				model.setProperty("/capacity", results[1] || []);
				model.setProperty("/movements", results[2] || []);
			}).catch(function () { /* reported */ });
		},

		onTransfer: function () {
			// A transfer moves stock between two locations of the same company;
			// the server refuses anything that would cross a company boundary.
			this.toast("transferHint");
		}
	});
});
