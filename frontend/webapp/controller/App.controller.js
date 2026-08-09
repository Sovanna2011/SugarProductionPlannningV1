sap.ui.define([
	"sugar/planning/controller/BaseController"
], function (BaseController) {
	"use strict";

	return BaseController.extend("sugar.planning.controller.App", {
		onInit: function () {
			this.getView().addStyleClass(
				this.getOwnerComponent().getContentDensityClass());
		}
	});
});
