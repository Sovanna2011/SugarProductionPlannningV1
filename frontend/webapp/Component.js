sap.ui.define([
	"sap/ui/core/UIComponent",
	"sap/ui/Device",
	"sugar/planning/model/models",
	"sugar/planning/service/ApiClient"
], function (UIComponent, Device, models, ApiClient) {
	"use strict";

	return UIComponent.extend("sugar.planning.Component", {

		metadata: { manifest: "json" },

		init: function () {
			UIComponent.prototype.init.apply(this, arguments);

			this.setModel(models.createDeviceModel(), "device");

			// The app model holds the signed-in user and the companies they may
			// work in. It deliberately does NOT hold a "current company": §9
			// requires the company to be chosen per view, which is what lets two
			// browser tabs work on two companies at once.
			this.setModel(models.createAppModel(), "app");
			this.refreshProfile();

			// A session that expires anywhere in the app lands back on login.
			ApiClient.onSessionExpired = function () {
				this.getModel("app").setProperty("/profile", null);
				this.getRouter().navTo("login", {}, true);
			}.bind(this);

			this.getRouter().initialize();

			if (!ApiClient.isLoggedIn()) {
				this.getRouter().navTo("login", {}, true);
			}
		},

		/** refreshProfile republishes the cached profile into the app model. */
		refreshProfile: function () {
			var profile = ApiClient.getProfile();
			var model = this.getModel("app");
			model.setProperty("/profile", profile);
			model.setProperty("/companies", profile ? profile.companies : []);
			model.setProperty("/userName", profile ? profile.user.fullName : "");
		},

		getContentDensityClass: function () {
			if (!this._contentDensityClass) {
				this._contentDensityClass = Device.support.touch
					? "sapUiSizeCozy"
					: "sapUiSizeCompact";
			}
			return this._contentDensityClass;
		}
	});
});
