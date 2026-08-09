sap.ui.define([
	"sugar/planning/controller/BaseController"
], function (BaseController) {
	"use strict";

	return BaseController.extend("sugar.planning.controller.Login", {

		onInit: function () {
			this.initViewModel({ username: "", password: "", error: "" });
		},

		onLogin: function () {
			var model = this.getViewModel();
			var that = this;

			model.setProperty("/error", "");

			this.api.post("/auth/login", {
				username: model.getProperty("/username"),
				password: model.getProperty("/password")
			}, { quiet: true }).then(function (result) {
				// The login response already carries every authorised company
				// with its own permission set, so the shell needs no further
				// round trip to render the company drop-downs (§11).
				that.api.setSession(result.tokens, {
					user: result.user,
					companies: result.companies
				});
				that.getOwnerComponent().refreshProfile();
				model.setProperty("/password", "");

				if (result.mustChangePassword) {
					that.toast("mustChangePassword");
				}
				that.getRouter().navTo("home", {}, true);
			}).catch(function (error) {
				model.setProperty("/error", error.message || that.i18n("loginFailed"));
			});
		}
	});
});
