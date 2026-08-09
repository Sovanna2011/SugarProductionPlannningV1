sap.ui.define([
	"sugar/planning/controller/BaseController"
], function (BaseController) {
	"use strict";

	return BaseController.extend("sugar.planning.controller.PlanVsActual", {

		permissions: [
			{ name: "View", permission: "REPORT.PLANACTUAL.VIEW" }
		],

		onInit: function () {
			this.initViewModel({
				seasons: [], versions: [], groupBy: "material",
				dateFrom: this._today(-30), dateTo: this._today(30),
				report: { lines: [], totalPlan: "0", totalActual: "0", totalVariance: "0" }
			});
			this.getRouter().getRoute("planVsActual").attachPatternMatched(this._onDisplay, this);
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
					// Default to the latest approved version, which is what the
					// business means by "the plan" (§F6).
					var approved = (versions || []).filter(function (version) {
						return version.status === "APPROVED" || version.status === "LOCKED";
					}).sort(function (a, b) { return b.versionNo - a.versionNo; })[0];
					model.setProperty("/versionId", approved ? String(approved.id) : null);
				}).catch(function () { /* reported */ });
		},

		onRun: function () {
			if (!this.requireCompany()) {
				return;
			}
			var model = this.getViewModel();

			var query = this.api.buildQuery({
				companyId: this.getCompanyId(),
				seasonId: model.getProperty("/seasonId"),
				versionId: model.getProperty("/versionId"),
				latestApproved: model.getProperty("/versionId") ? null : "true",
				groupBy: model.getProperty("/groupBy"),
				dateFrom: model.getProperty("/dateFrom"),
				dateTo: model.getProperty("/dateTo")
			});

			this.api.get("/reports/plan-vs-actual" + query).then(function (report) {
				model.setProperty("/report", report || { lines: [] });
			}).catch(function () { /* reported */ });
		},

		_today: function (offsetDays) {
			var date = new Date();
			date.setUTCDate(date.getUTCDate() + offsetDays);
			return date.toISOString().substring(0, 10);
		}
	});
});
