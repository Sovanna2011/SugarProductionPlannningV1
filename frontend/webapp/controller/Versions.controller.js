sap.ui.define([
	"sugar/planning/controller/BaseController",
	"sap/m/Dialog",
	"sap/m/Button",
	"sap/m/Input",
	"sap/m/Label",
	"sap/m/VBox"
], function (BaseController, Dialog, Button, Input, Label, VBox) {
	"use strict";

	return BaseController.extend("sugar.planning.controller.Versions", {

		permissions: [
			{ name: "Create", permission: "PLAN.VERSION.CREATE" },
			{ name: "Copy", permission: "PLAN.VERSION.COPY" },
			{ name: "Submit", permission: "PLAN.VERSION.SUBMIT" },
			{ name: "Approve", permission: "PLAN.VERSION.APPROVE" },
			{ name: "Lock", permission: "PLAN.VERSION.LOCK" }
		],

		onInit: function () {
			this.initViewModel({ seasons: [], versions: [] });
			this.getRouter().getRoute("versions").attachPatternMatched(this._onDisplay, this);
		},

		_onDisplay: function () {
			if (!this.api.isLoggedIn()) {
				this.getRouter().navTo("login", {}, true);
				return;
			}
			this.applyDefaultCompany();
			this._loadSeasons();
		},

		onCompanySelected: function () {
			this.getViewModel().setProperty("/versions", []);
			this._loadSeasons();
		},

		_loadSeasons: function () {
			var companyId = this.getCompanyId();
			if (!companyId) {
				return;
			}
			var model = this.getViewModel();
			var that = this;

			this.api.get("/companies/" + companyId + "/seasons?size=100").then(function (seasons) {
				model.setProperty("/seasons", seasons || []);
				var open = (seasons || []).filter(function (s) { return s.status === "OPEN"; })[0];
				if (open) {
					model.setProperty("/seasonId", String(open.id));
					that.onSeasonChange();
				}
			}).catch(function () { /* reported */ });
		},

		onSeasonChange: function () {
			var companyId = this.getCompanyId();
			var seasonId = this.getViewModel().getProperty("/seasonId");
			if (!companyId || !seasonId) {
				return;
			}
			var model = this.getViewModel();
			this.api.get("/companies/" + companyId + "/seasons/" + seasonId + "/planning-versions")
				.then(function (versions) {
					model.setProperty("/versions", versions || []);
				}).catch(function () { /* reported */ });
		},

		onCreate: function () {
			var that = this;
			this._prompt("newVersion", "", function (name) {
				that.api.post("/companies/" + that.getCompanyId() + "/seasons/" +
					that.getViewModel().getProperty("/seasonId") + "/planning-versions", {
						seasonId: Number(that.getViewModel().getProperty("/seasonId")),
						versionName: name
					}).then(function () {
						that.toast("versionCreated");
						that.onSeasonChange();
					}).catch(function () { /* reported */ });
			});
		},

		/**
		 * onCopy produces a deep, independent snapshot. The new version can be
		 * edited freely without touching the one it came from (§31).
		 */
		onCopy: function (event) {
			var version = event.getSource().getBindingContext("view").getObject();
			var that = this;

			this._prompt("copyVersion", version.versionName + " (copy)", function (name) {
				that.api.post("/planning-versions/" + version.id + "/copy?companyId=" +
					that.getCompanyId(), { versionName: name })
					.then(function () {
						that.toast("versionCopied");
						that.onSeasonChange();
					}).catch(function () { /* reported */ });
			});
		},

		onSubmit:  function (e) { this._transition(e, "submit", "versionSubmitted"); },
		onApprove: function (e) { this._transition(e, "approve", "versionApproved"); },
		onLock:    function (e) { this._transition(e, "lock", "versionLocked"); },

		_transition: function (event, action, message) {
			var version = event.getSource().getBindingContext("view").getObject();
			var that = this;

			this.api.post("/planning-versions/" + version.id + "/" + action +
				"?companyId=" + this.getCompanyId(), {})
				.then(function () {
					that.toast(message);
					that.onSeasonChange();
				}).catch(function () { /* reported */ });
		},

		_prompt: function (titleKey, initial, onConfirm) {
			var input = new Input({ value: initial, width: "100%" });
			var dialog = new Dialog({
				title: this.i18n(titleKey),
				contentWidth: "24rem",
				content: [new VBox({
					items: [new Label({ text: this.i18n("versionName") }), input]
				}).addStyleClass("sapUiSmallMargin")],
				beginButton: new Button({
					text: this.i18n("ok"),
					type: "Emphasized",
					press: function () {
						dialog.close();
						onConfirm(input.getValue());
					}
				}),
				endButton: new Button({
					text: this.i18n("cancel"),
					press: function () { dialog.close(); }
				}),
				afterClose: function () { dialog.destroy(); }
			});
			this.getView().addDependent(dialog);
			dialog.open();
		}
	});
});
