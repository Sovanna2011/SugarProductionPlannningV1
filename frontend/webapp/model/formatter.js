sap.ui.define([], function () {
	"use strict";

	var QUANTITY = new Intl.NumberFormat(undefined, {
		minimumFractionDigits: 3,
		maximumFractionDigits: 3
	});

	var PERCENT = new Intl.NumberFormat(undefined, {
		minimumFractionDigits: 2,
		maximumFractionDigits: 2
	});

	return {
		/** Quantities arrive as strings so a NUMERIC(18,3) keeps its precision. */
		quantity: function (value) {
			if (value === null || value === undefined || value === "") {
				return "";
			}
			var parsed = Number(value);
			return isNaN(parsed) ? String(value) : QUANTITY.format(parsed);
		},

		/**
		 * A null variance percentage means "undefined", not zero: it is what
		 * the server sends for production against a zero plan (§F6).
		 */
		variancePercent: function (value) {
			if (value === null || value === undefined) {
				return "n/a";
			}
			var parsed = Number(value);
			return isNaN(parsed) ? "n/a" : PERCENT.format(parsed) + " %";
		},

		/** Semantic colour for a variance: short of plan is the warning case. */
		varianceState: function (value) {
			if (value === null || value === undefined) {
				return "None";
			}
			var parsed = Number(value);
			if (isNaN(parsed)) {
				return "None";
			}
			if (parsed < -10) {
				return "Error";
			}
			if (parsed < 0) {
				return "Warning";
			}
			return "Success";
		},

		/** Traffic light of §F5 mapped onto the Fiori progress states. */
		capacityState: function (trafficLight) {
			switch (trafficLight) {
				case "RED":
					return "Error";
				case "AMBER":
					return "Warning";
				case "GREEN":
					return "Success";
				default:
					return "None";
			}
		},

		capacityValue: function (utilisation) {
			if (utilisation === null || utilisation === undefined) {
				return 0;
			}
			return Math.min(100, Number(utilisation) || 0);
		},

		/** An unlimited location shows a dash rather than 0 %. */
		capacityText: function (utilisation) {
			if (utilisation === null || utilisation === undefined) {
				return "unlimited";
			}
			return PERCENT.format(Number(utilisation)) + " %";
		},

		versionState: function (status) {
			switch (status) {
				case "APPROVED":
					return "Success";
				case "LOCKED":
					return "Information";
				case "SUBMITTED":
					return "Warning";
				case "CANCELLED":
					return "Error";
				default:
					return "None";
			}
		},

		postingState: function (status) {
			switch (status) {
				case "POSTED":
					return "Success";
				case "REVERSED":
					return "Error";
				default:
					return "Warning";
			}
		},

		date: function (value) {
			if (!value) {
				return "";
			}
			return value.substring(0, 10);
		}
	};
});
