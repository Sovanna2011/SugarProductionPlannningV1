sap.ui.define([
	"sap/m/MessageBox",
	"sap/ui/core/BusyIndicator"
], function (MessageBox, BusyIndicator) {
	"use strict";

	/**
	 * ApiClient is the single fetch wrapper of §H4. Everything that talks to
	 * the backend goes through it, which is what keeps token handling, the
	 * request id, the busy indicator and error reporting in one place.
	 *
	 * It deliberately holds no company: §9 forbids a global company context,
	 * so each call carries its own companyId supplied by the calling view.
	 */
	var TOKEN_KEY = "sugar.accessToken";
	var REFRESH_KEY = "sugar.refreshToken";
	var PROFILE_KEY = "sugar.profile";

	var ApiClient = {

		baseUrl: "/api/v1",

		// --- token handling ----------------------------------------------

		setSession: function (tokens, profile) {
			sessionStorage.setItem(TOKEN_KEY, tokens.accessToken);
			sessionStorage.setItem(REFRESH_KEY, tokens.refreshToken);
			sessionStorage.setItem(PROFILE_KEY, JSON.stringify(profile));
		},

		clearSession: function () {
			sessionStorage.removeItem(TOKEN_KEY);
			sessionStorage.removeItem(REFRESH_KEY);
			sessionStorage.removeItem(PROFILE_KEY);
		},

		getToken: function () {
			return sessionStorage.getItem(TOKEN_KEY);
		},

		isLoggedIn: function () {
			return !!this.getToken();
		},

		/**
		 * getProfile returns the user together with every company they are
		 * authorised for. The company drop-down of each view is filled from
		 * here — the profile is a list of options, never a current selection.
		 */
		getProfile: function () {
			var raw = sessionStorage.getItem(PROFILE_KEY);
			return raw ? JSON.parse(raw) : null;
		},

		/**
		 * hasPermission answers "may this user do X in company Y". The answer
		 * differs per company for the same user (§13), so the company is a
		 * mandatory argument — there is no per-user overload on purpose.
		 */
		hasPermission: function (companyId, permission) {
			var profile = this.getProfile();
			if (!profile) {
				return false;
			}
			var company = (profile.companies || []).filter(function (entry) {
				return entry.companyId === companyId;
			})[0];
			return !!company && company.permissions.indexOf(permission) >= 0;
		},

		// --- requests ------------------------------------------------------

		get: function (path, options) {
			return this.request("GET", path, null, options);
		},

		post: function (path, body, options) {
			return this.request("POST", path, body, options);
		},

		put: function (path, body, options) {
			return this.request("PUT", path, body, options);
		},

		del: function (path, options) {
			return this.request("DELETE", path, null, options);
		},

		/**
		 * request performs the call, refreshing the access token once on 401
		 * before giving up.
		 */
		request: function (method, path, body, options) {
			var that = this;
			options = options || {};

			if (!options.silent) {
				BusyIndicator.show(0);
			}

			return this._send(method, path, body, options)
				.then(function (response) {
					if (response.status === 401 && !options._retried) {
						return that._refresh().then(function (refreshed) {
							if (!refreshed) {
								that._forceLogin();
								return Promise.reject(new Error("session expired"));
							}
							options._retried = true;
							return that._send(method, path, body, options);
						});
					}
					return response;
				})
				.then(function (response) {
					return that._handle(response, options);
				})
				.finally(function () {
					if (!options.silent) {
						BusyIndicator.hide();
					}
				});
		},

		_send: function (method, path, body, options) {
			var headers = {
				"Accept": "application/json",
				"X-Request-Id": this._requestId()
			};
			if (body !== null && body !== undefined) {
				headers["Content-Type"] = "application/json";
			}
			var token = this.getToken();
			if (token) {
				headers.Authorization = "Bearer " + token;
			}
			if (options.idempotencyKey) {
				headers["Idempotency-Key"] = options.idempotencyKey;
			}

			return fetch(this.baseUrl + path, {
				method: method,
				headers: headers,
				body: body !== null && body !== undefined ? JSON.stringify(body) : undefined
			});
		},

		_handle: function (response, options) {
			var that = this;

			if (response.status === 204) {
				return Promise.resolve(null);
			}

			return response.text().then(function (text) {
				var payload = null;
				if (text) {
					try {
						payload = JSON.parse(text);
					} catch (e) {
						payload = null;
					}
				}

				if (response.ok) {
					return options.withMeta ? payload : (payload ? payload.data : null);
				}

				var error = (payload && payload.error) || {
					code: "E-GEN-" + response.status,
					message: "The request failed"
				};
				error.status = response.status;
				error.requestId = payload && payload.requestId;

				if (response.status === 401) {
					that._forceLogin();
				} else if (!options.quiet) {
					that.showError(error);
				}
				return Promise.reject(error);
			});
		},

		/**
		 * showError reports the business error code alongside the message, so
		 * a user can quote it in a support call and it can be traced through
		 * the audit log by its request id.
		 */
		showError: function (error) {
			var details = "";
			if (error.details && error.details.length) {
				details = error.details.map(function (detail) {
					return "• " + [detail.field, detail.value, detail.message]
						.filter(Boolean).join(": ");
				}).join("\n");
			}

			MessageBox.error(error.message || "The request failed", {
				title: error.code || "Error",
				details: [details, error.requestId ? "Request " + error.requestId : ""]
					.filter(Boolean).join("\n\n")
			});
		},

		_refresh: function () {
			var that = this;
			var refreshToken = sessionStorage.getItem(REFRESH_KEY);
			if (!refreshToken) {
				return Promise.resolve(false);
			}

			return fetch(this.baseUrl + "/auth/refresh", {
				method: "POST",
				headers: { "Content-Type": "application/json" },
				body: JSON.stringify({ refreshToken: refreshToken })
			}).then(function (response) {
				if (!response.ok) {
					return false;
				}
				return response.json().then(function (payload) {
					sessionStorage.setItem(TOKEN_KEY, payload.data.accessToken);
					sessionStorage.setItem(REFRESH_KEY, payload.data.refreshToken);
					return true;
				});
			}).catch(function () {
				return false;
			});
		},

		_forceLogin: function () {
			this.clearSession();
			if (this.onSessionExpired) {
				this.onSessionExpired();
			}
		},

		_requestId: function () {
			if (window.crypto && window.crypto.randomUUID) {
				return window.crypto.randomUUID();
			}
			return "req-" + Date.now() + "-" + Math.random().toString(16).slice(2);
		},

		/**
		 * buildQuery turns an object into a query string, dropping empty
		 * values so an unset filter never reaches the server as "undefined".
		 */
		buildQuery: function (params) {
			var parts = [];
			Object.keys(params || {}).forEach(function (key) {
				var value = params[key];
				if (value === null || value === undefined || value === "") {
					return;
				}
				parts.push(encodeURIComponent(key) + "=" + encodeURIComponent(value));
			});
			return parts.length ? "?" + parts.join("&") : "";
		}
	};

	return ApiClient;
});
