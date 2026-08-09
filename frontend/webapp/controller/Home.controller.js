sap.ui.define([
	"sugar/planning/controller/BaseController"
], function (BaseController) {
	"use strict";

	return BaseController.extend("sugar.planning.controller.Home", {

		onInit: function () {
			this.initViewModel();
			this.getRouter().getRoute("home").attachPatternMatched(function () {
				if (!this.api.isLoggedIn()) {
					this.getRouter().navTo("login", {}, true);
					return;
				}
				this.getOwnerComponent().refreshProfile();
			}, this);
		},

		onOpenPlanning:     function () { this.navTo("planning"); },
		onOpenVersions:     function () { this.navTo("versions"); },
		onOpenActual:       function () { this.navTo("actual"); },
		onOpenInventory:    function () { this.navTo("inventory"); },
		onOpenPlanVsActual: function () { this.navTo("planVsActual"); },
		onOpenBrowser:      function () { this.navTo("browser"); },
		onOpenDictionary:   function () { this.navTo("dictionary"); }
	});
});
