sap.ui.define([
	"sap/ui/model/json/JSONModel",
	"sap/ui/Device"
], function (JSONModel, Device) {
	"use strict";

	return {
		createDeviceModel: function () {
			var model = new JSONModel(Device);
			model.setDefaultBindingMode("OneWay");
			return model;
		},

		/**
		 * The application model carries the signed-in user and the list of
		 * companies they are authorised for. There is no "current company"
		 * here on purpose (§9, §H2) — each view keeps its own selection.
		 */
		createAppModel: function () {
			return new JSONModel({
				profile: null,
				companies: [],
				userName: ""
			});
		},

		/**
		 * createViewModel builds the per-view model that holds that view's own
		 * company selection and filter state. Because it lives on the view and
		 * not on the component, two tabs can work on two companies with no
		 * interference at all.
		 */
		createViewModel: function (initial) {
			return new JSONModel(Object.assign({
				companyId: null,
				busy: false,
				editable: false
			}, initial || {}));
		}
	};
});
