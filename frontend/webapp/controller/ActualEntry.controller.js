sap.ui.define([
	"sugar/planning/controller/BaseController",
	"sap/ui/core/Fragment"
], function (BaseController, Fragment) {
	"use strict";

	return BaseController.extend("sugar.planning.controller.ActualEntry", {

		permissions: [
			{ name: "Create", permission: "ACTUAL.CREATE" },
			{ name: "Post", permission: "ACTUAL.POST" },
			{ name: "Reverse", permission: "ACTUAL.REVERSE" }
		],

		onInit: function () {
			this.initViewModel({
				documents: [], materials: [], warehouses: [], movementTypes: [],
				lines: [], processes: [], message: "", messageType: "Information",
				dateFrom: this._today(-7), dateTo: this._today(1),
				entry: this._emptyEntry()
			});
			this.getRouter().getRoute("actual").attachPatternMatched(this._onDisplay, this);
		},

		_onDisplay: function () {
			if (!this.api.isLoggedIn()) {
				this.getRouter().navTo("login", {}, true);
				return;
			}
			this.applyDefaultCompany();
			this._loadContext();
			this.onLoad();
		},

		onCompanySelected: function () {
			this.getViewModel().setProperty("/documents", []);
			this._loadContext();
			this.onLoad();
		},

		_loadContext: function () {
			var companyId = this.getCompanyId();
			if (!companyId) {
				return;
			}
			var model = this.getViewModel();

			Promise.all([
				this.api.get("/companies/" + companyId + "/materials?size=200"),
				this.api.get("/companies/" + companyId + "/warehouses?size=200"),
				this.api.get("/movement-types?size=100&companyId=" + companyId),
				this.api.get("/companies/" + companyId + "/production-lines?size=100"),
				this.api.get("/processes?size=100&companyId=" + companyId)
			]).then(function (results) {
				model.setProperty("/materials", (results[0] || [])
					.filter(function (entry) { return entry.productionEnabled; })
					.map(function (entry) { return entry.material; })
					.filter(Boolean));
				model.setProperty("/warehouses", results[1] || []);
				model.setProperty("/movementTypes", results[2] || []);
				model.setProperty("/lines", results[3] || []);
				model.setProperty("/processes", results[4] || []);
			}).catch(function () { /* reported */ });
		},

		onLoad: function () {
			var companyId = this.getCompanyId();
			if (!companyId) {
				return;
			}
			var model = this.getViewModel();

			var query = this.api.buildQuery({
				companyId: companyId,
				dateFrom: model.getProperty("/dateFrom"),
				dateTo: model.getProperty("/dateTo"),
				size: 100
			});

			this.api.get("/actuals" + query).then(function (documents) {
				model.setProperty("/documents", documents || []);
			}).catch(function () { /* reported */ });
		},

		onNew: function () {
			var that = this;
			var model = this.getViewModel();
			model.setProperty("/entry", this._emptyEntry());

			this._openDialog().then(function (dialog) {
				dialog.open();
			});
		},

		_openDialog: function () {
			var that = this;
			if (!this._dialog) {
				this._dialog = Fragment.load({
					id: this.getView().getId(),
					name: "sugar.planning.fragment.ActualDialog",
					controller: this
				}).then(function (dialog) {
					that.getView().addDependent(dialog);
					return dialog;
				});
			}
			return this._dialog;
		},

		onDialogCancel: function () {
			this._openDialog().then(function (dialog) { dialog.close(); });
		},

		/**
		 * onDialogSave creates the draft. An Idempotency-Key makes a retry
		 * after a timeout return the original document rather than a second
		 * one (§E8).
		 */
		onDialogSave: function () {
			var model = this.getViewModel();
			var entry = model.getProperty("/entry");
			var that = this;

			var payload = {
				movementTypeId: Number(entry.movementTypeId),
				postingDate: entry.postingDate,
				description: entry.description,
				productionLineId: entry.productionLineId ? Number(entry.productionLineId) : null,
				items: [{
					actualDate: entry.postingDate,
					materialId: Number(entry.materialId),
					processId: entry.processId ? Number(entry.processId) : null,
					warehouseId: entry.warehouseId ? Number(entry.warehouseId) : null,
					productionLineId: entry.productionLineId ? Number(entry.productionLineId) : null,
					quantity: String(entry.quantity),
					uomId: Number(entry.uomId)
				}]
			};

			this.api.post("/actuals?companyId=" + this.getCompanyId(), payload, {
				idempotencyKey: "ui-" + Date.now() + "-" + Math.random().toString(16).slice(2)
			}).then(function () {
				that.onDialogCancel();
				that.toast("documentCreated");
				that.onLoad();
			}).catch(function () { /* reported */ });
		},

		/** Selecting a material fills the unit, which the server compares against. */
		onMaterialSelected: function () {
			var model = this.getViewModel();
			var materialId = model.getProperty("/entry/materialId");
			var material = (model.getProperty("/materials") || []).filter(function (entry) {
				return String(entry.id) === String(materialId);
			})[0];
			if (material) {
				model.setProperty("/entry/uomId", material.baseUomId);
			}
		},

		onPost: function (event) {
			var document = event.getSource().getBindingContext("view").getObject();
			var that = this;

			this.api.post("/actuals/" + document.id + "/post?companyId=" + this.getCompanyId(), {})
				.then(function () {
					that._setMessage(that.i18n("documentPosted", [document.documentNo]), "Success");
					that.onLoad();
				})
				.catch(function (error) {
					// A refusal here is a business rule doing its job — the
					// routing rules of §F7, a capacity limit or negative stock.
					that._setMessage((error.code || "") + " " + (error.message || ""), "Error");
				});
		},

		onReverse: function (event) {
			var document = event.getSource().getBindingContext("view").getObject();
			var that = this;

			this.api.post("/actuals/" + document.id + "/reverse?companyId=" + this.getCompanyId(),
				{ remark: "Reversed from the production entry screen" })
				.then(function () {
					that._setMessage(that.i18n("documentReversed", [document.documentNo]), "Success");
					that.onLoad();
				}).catch(function () { /* reported */ });
		},

		_setMessage: function (text, type) {
			this.getViewModel().setProperty("/message", text);
			this.getViewModel().setProperty("/messageType", type);
		},

		_emptyEntry: function () {
			return {
				movementTypeId: null, postingDate: this._today(0), description: "",
				materialId: null, processId: null, warehouseId: null,
				productionLineId: null, quantity: "0", uomId: null
			};
		},

		_today: function (offsetDays) {
			var date = new Date();
			date.setUTCDate(date.getUTCDate() + offsetDays);
			return date.toISOString().substring(0, 10);
		}
	});
});
