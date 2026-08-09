sap.ui.define([
	"sap/ui/core/mvc/Controller",
	"sap/ui/core/UIComponent",
	"sap/m/MessageToast",
	"sugar/planning/service/ApiClient",
	"sugar/planning/model/models",
	"sugar/planning/model/formatter"
], function (Controller, UIComponent, MessageToast, ApiClient, models, formatter) {
	"use strict";

	return Controller.extend("sugar.planning.controller.BaseController", {

		formatter: formatter,
		api: ApiClient,

		getRouter: function () {
			return UIComponent.getRouterFor(this);
		},

		getAppModel: function () {
			return this.getOwnerComponent().getModel("app");
		},

		i18n: function (key, args) {
			return this.getOwnerComponent().getModel("i18n")
				.getResourceBundle().getText(key, args);
		},

		/**
		 * initViewModel creates the model that holds this view's own company
		 * selection (§H2). The selection lives here and nowhere else — there is
		 * no component-level or browser-storage company, which is exactly what
		 * makes two tabs on two companies possible.
		 */
		initViewModel: function (initial) {
			var model = models.createViewModel(initial);
			this.getView().setModel(model, "view");
			return model;
		},

		getViewModel: function () {
			return this.getView().getModel("view");
		},

		/** The company this view is currently working on. */
		getCompanyId: function () {
			var companyId = this.getViewModel().getProperty("/companyId");
			return companyId ? Number(companyId) : null;
		},

		/**
		 * defaultCompany pre-selects the user's default company but leaves it
		 * freely changeable, per §11.
		 */
		defaultCompany: function () {
			var companies = this.getAppModel().getProperty("/companies") || [];
			var preferred = companies.filter(function (entry) {
				return entry.isDefault;
			})[0] || companies[0];
			return preferred ? preferred.companyId : null;
		},

		/**
		 * applyDefaultCompany fills the view's company selection on first
		 * display, without overwriting a choice the user has already made.
		 */
		applyDefaultCompany: function () {
			var model = this.getViewModel();
			if (!model.getProperty("/companyId")) {
				model.setProperty("/companyId", this.defaultCompany());
			}
			this.refreshPermissions();
		},

		/**
		 * refreshPermissions re-evaluates what the user may do — in the company
		 * this view is pointed at. Switching the drop-down can turn an editable
		 * screen into a display-only one for the very same user (§13).
		 */
		refreshPermissions: function (permissions) {
			var companyId = this.getCompanyId();
			var model = this.getViewModel();
			var api = this.api;

			(permissions || this.permissions || []).forEach(function (entry) {
				model.setProperty("/can" + entry.name,
					companyId ? api.hasPermission(companyId, entry.permission) : false);
			});
		},

		onCompanyChange: function () {
			this.refreshPermissions();
			if (this.onCompanySelected) {
				this.onCompanySelected();
			}
		},

		requireCompany: function () {
			if (!this.getCompanyId()) {
				MessageToast.show(this.i18n("selectCompanyFirst"));
				return false;
			}
			return true;
		},

		toast: function (key, args) {
			MessageToast.show(this.i18n(key, args));
		},

		navTo: function (route) {
			this.getRouter().navTo(route);
		},

		onNavBack: function () {
			this.getRouter().navTo("home");
		},

		onLogout: function () {
			var that = this;
			this.api.post("/auth/logout", {}, { quiet: true })
				.catch(function () { /* logging out locally is enough */ })
				.finally(function () {
					that.api.clearSession();
					that.getOwnerComponent().refreshProfile();
					that.getRouter().navTo("login", {}, true);
				});
		}
	});
});
