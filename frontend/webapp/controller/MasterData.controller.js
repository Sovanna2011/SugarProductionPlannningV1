sap.ui.define([
	"sugar/planning/controller/BaseController",
	"sap/m/Dialog",
	"sap/m/Button",
	"sap/m/Label",
	"sap/m/Input",
	"sap/m/Select",
	"sap/m/CheckBox",
	"sap/ui/core/Item",
	"sap/ui/layout/form/SimpleForm",
	"sap/m/MessageBox"
], function (BaseController, Dialog, Button, Label, Input, Select, CheckBox,
	Item, SimpleForm, MessageBox) {
	"use strict";

	/**
	 * MasterData maintains company, material, warehouse, condition silo and
	 * tank.
	 *
	 * The last three share one definition because they share one table: a tank
	 * and a condition silo are warehouses whose type differs, and giving them
	 * separate screens would have meant duplicating the capacity rule, the
	 * optimistic lock and the audit trail three times over. What differs
	 * between them is the type they create and the list they read.
	 */
	return BaseController.extend("sugar.planning.controller.MasterData", {

		permissions: [
			{ name: "EditCompany", permission: "MASTER.COMPANY.EDIT" },
			{ name: "EditMaterial", permission: "MASTER.MATERIAL.EDIT" },
			{ name: "EditWarehouse", permission: "MASTER.WAREHOUSE.EDIT" }
		],

		/**
		 * _objects describes each maintained object once: where to read it,
		 * where to write it, which permission guards it, and which fields the
		 * dialog offers. Everything else in this controller is generic.
		 */
		_objects: {
			company: {
				list: function () { return "/companies?size=200&companyId=" + this.getCompanyId(); },
				save: function () { return "/companies?companyId=" + this.getCompanyId(); },
				update: function (row) { return "/companies/" + row.id; },
				permission: "MASTER.COMPANY.EDIT",
				code: "companyCode", name: "companyName",
				attribute: "localCurrency", attributeLabel: "masterCurrency",
				fields: [
					{ key: "companyCode", label: "code", required: true },
					{ key: "companyName", label: "name", required: true },
					{ key: "localCurrency", label: "masterCurrency", required: true, value: "USD" },
					{ key: "groupCurrency", label: "masterGroupCurrency", required: true, value: "USD" },
					{ key: "timezone", label: "masterTimezone", required: true, value: "UTC" }
				]
			},
			material: {
				list: function () { return "/materials?size=200&companyId=" + this.getCompanyId(); },
				save: function () { return "/materials?companyId=" + this.getCompanyId(); },
				update: function (row) { return "/materials/" + row.id + "?companyId=" + this.getCompanyId(); },
				remove: function (row) {
					return "/materials/" + row.id + "?companyId=" + this.getCompanyId() +
						"&version=" + row.version;
				},
				permission: "MASTER.MATERIAL.EDIT",
				code: "materialCode", name: "materialName",
				attribute: "materialType", attributeLabel: "masterType",
				fields: [
					{ key: "materialCode", label: "code", required: true },
					{ key: "materialName", label: "name", required: true },
					{ key: "materialType", label: "masterType", required: true,
						choices: ["RAW", "SEMI_FINISHED", "FINISHED", "BY_PRODUCT", "UTILITY"] },
					{ key: "materialGroup", label: "masterGroup" },
					{ key: "baseUomId", label: "masterBaseUom", required: true, source: "uoms",
						sourceText: "uomCode" },
					// §F7 routing, expressed as data: a grade that needs
					// conditioning goes through the silo, one that does not
					// never enters it.
					{ key: "conditioningRequired", label: "masterConditioning", type: "boolean" },
					{ key: "isStockManaged", label: "masterStockManaged", type: "boolean", value: true }
				]
			},
			warehouse: {
				list: function () { return "/companies/" + this.getCompanyId() + "/warehouses?size=200"; },
				save: function () { return "/companies/" + this.getCompanyId() + "/warehouses"; },
				update: function (row) { return "/companies/" + this.getCompanyId() + "/warehouses/" + row.id; },
				remove: function (row) {
					return "/companies/" + this.getCompanyId() + "/warehouses/" + row.id +
						"?version=" + row.version;
				},
				permission: "MASTER.WAREHOUSE.EDIT",
				code: "warehouseCode", name: "warehouseName",
				attribute: "warehouseType", attributeLabel: "masterType",
				warehouseType: "WAREHOUSE",
				fields: [
					{ key: "warehouseCode", label: "code", required: true },
					{ key: "warehouseName", label: "name", required: true },
					{ key: "warehouseType", label: "masterType", required: true,
						choices: ["WAREHOUSE", "TANK", "SILO", "PRODUCTION_STORAGE"] },
					// A null capacity means unlimited — not zero (§F5).
					{ key: "capacity", label: "capacity", hint: "masterCapacityHint" },
					{ key: "capacityUomId", label: "masterCapacityUom", source: "uoms", sourceText: "uomCode" },
					{ key: "location", label: "masterLocation" },
					{ key: "allowNegativeStock", label: "masterAllowNegative", type: "boolean" }
				]
			}
		},

		onInit: function () {
			this.initViewModel({
				tab: "company", rows: [], uoms: [], search: "",
				tabTitle: "", attributeLabel: "", canEdit: false
			});
			this.getRouter().getRoute("masterData").attachPatternMatched(this._onDisplay, this);
		},

		_onDisplay: function () {
			if (!this.api.isLoggedIn()) {
				this.getRouter().navTo("login", {}, true);
				return;
			}
			this.applyDefaultCompany();
			this._loadUnits();
			this._reload();
		},

		onCompanySelected: function () {
			this._reload();
		},

		onTabSelect: function (event) {
			this.getViewModel().setProperty("/tab", event.getParameter("key"));
			this._reload();
		},

		onSearch: function () {
			this._reload();
		},

		_loadUnits: function () {
			var model = this.getViewModel();
			this.api.get("/uoms?size=100&companyId=" + this.getCompanyId())
				.then(function (uoms) { model.setProperty("/uoms", uoms || []); })
				.catch(function () { /* reported */ });
		},

		/**
		 * _definition resolves the tab to its object description. Silos and
		 * tanks reuse the warehouse definition with a different list endpoint
		 * and a different type to create — they are not a separate entity.
		 */
		_definition: function () {
			var tab = this.getViewModel().getProperty("/tab");
			if (tab === "silo" || tab === "tank") {
				var warehouse = Object.create(this._objects.warehouse);
				warehouse.warehouseType = tab === "silo" ? "SILO" : "TANK";
				warehouse.list = function () {
					return "/companies/" + this.getCompanyId() +
						(tab === "silo" ? "/condition-silos" : "/tanks") + "?size=200";
				};
				return warehouse;
			}
			return this._objects[tab] || this._objects.company;
		},

		_reload: function () {
			var companyId = this.getCompanyId();
			if (!companyId) {
				return;
			}
			var model = this.getViewModel();
			var definition = this._definition();
			var tab = model.getProperty("/tab");
			var search = (model.getProperty("/search") || "").toLowerCase();
			var that = this;

			model.setProperty("/canEdit", this.api.hasPermission(companyId, definition.permission));
			model.setProperty("/tabTitle", this.i18n("master" + tab.charAt(0).toUpperCase() + tab.slice(1)));
			model.setProperty("/attributeLabel", this.i18n(definition.attributeLabel));

			this.api.get(definition.list.call(this)).then(function (rows) {
				var mapped = (rows || []).map(function (row) {
					return Object.assign({}, row, {
						code: row[definition.code],
						name: row[definition.name],
						attribute: row[definition.attribute],
						capacityText: that._capacityText(row)
					});
				}).filter(function (row) {
					return !search ||
						(row.code || "").toLowerCase().indexOf(search) >= 0 ||
						(row.name || "").toLowerCase().indexOf(search) >= 0;
				});
				model.setProperty("/rows", mapped);
			}).catch(function () { /* reported */ });
		},

		/** A location with no maintained capacity is unlimited, not zero. */
		_capacityText: function (row) {
			if (row.capacity === null || row.capacity === undefined) {
				return row.warehouseCode ? this.i18n("unlimited") : "";
			}
			return this.formatter.quantity(row.capacity);
		},

		// --- maintenance ------------------------------------------------------

		onCreate: function () {
			this._openDialog(null);
		},

		onEdit: function (event) {
			this._openDialog(event.getSource().getBindingContext("view").getObject());
		},

		/**
		 * _openDialog builds the form from the field descriptors, so adding a
		 * maintained attribute is one entry in _objects rather than a new
		 * dialog.
		 */
		_openDialog: function (row) {
			var definition = this._definition();
			var model = this.getViewModel();
			var values = {};
			var that = this;
			var form = new SimpleForm({ editable: true, layout: "ResponsiveGridLayout" });

			definition.fields.forEach(function (field) {
				var current = row ? row[field.key] : field.value;
				form.addContent(new Label({
					text: that.i18n(field.label),
					required: !!field.required,
					tooltip: field.hint ? that.i18n(field.hint) : null
				}));

				if (field.type === "boolean") {
					var box = new CheckBox({ selected: !!current });
					box.attachSelect(function (event) {
						values[field.key] = event.getParameter("selected");
					});
					values[field.key] = !!current;
					form.addContent(box);
					return;
				}

				if (field.choices || field.source) {
					var options = field.choices
						? field.choices.map(function (choice) {
							return new Item({ key: choice, text: choice });
						})
						: (model.getProperty("/" + field.source) || []).map(function (entry) {
							return new Item({ key: String(entry.id), text: entry[field.sourceText] });
						});

					var select = new Select({ items: options, width: "100%" });
					if (current !== null && current !== undefined) {
						select.setSelectedKey(String(current));
					}
					// The warehouse type of a silo or tank screen is fixed:
					// creating a tank from the tank screen must produce a tank.
					if (field.key === "warehouseType" && !row && definition.warehouseType) {
						select.setSelectedKey(definition.warehouseType);
					}
					values[field.key] = select.getSelectedKey();
					select.attachChange(function (event) {
						values[field.key] = event.getSource().getSelectedKey();
					});
					form.addContent(select);
					return;
				}

				var input = new Input({ value: current === null || current === undefined ? "" : String(current) });
				values[field.key] = input.getValue();
				input.attachChange(function (event) {
					values[field.key] = event.getParameter("value");
				});
				form.addContent(input);
			});

			var dialog = new Dialog({
				title: this.i18n(row ? "edit" : "create"),
				contentWidth: "32rem",
				content: [form],
				beginButton: new Button({
					text: this.i18n("save"),
					type: "Emphasized",
					press: function () {
						that._save(definition, row, values, dialog);
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
		},

		_save: function (definition, row, values, dialog) {
			var payload = {};
			var that = this;

			definition.fields.forEach(function (field) {
				var value = values[field.key];
				if (field.type === "boolean") {
					payload[field.key] = !!value;
					return;
				}
				if (value === "" || value === undefined || value === null) {
					// An omitted number stays absent rather than becoming zero:
					// an unmaintained capacity means unlimited.
					payload[field.key] = null;
					return;
				}
				payload[field.key] = field.key.endsWith("Id") ? Number(value) : value;
			});

			// The optimistic lock is part of the contract: an edit carries the
			// version it read, and a stale one is refused with 409.
			var request = row
				? this.api.put(definition.update.call(this, row),
					Object.assign(payload, { version: row.version }))
				: this.api.post(definition.save.call(this), payload);

			request.then(function () {
				dialog.close();
				that._reload();
				that.toast("saved");
			}).catch(function () { /* ApiClient already reported it */ });
		},

		/** Master data is deactivated, never deleted (§B1). */
		onDeactivate: function (event) {
			var row = event.getSource().getBindingContext("view").getObject();
			var definition = this._definition();
			var that = this;

			if (!definition.remove) {
				this.toast("masterNotDeactivatable");
				return;
			}

			MessageBox.confirm(this.i18n("masterConfirmDeactivate", [row.code]), {
				onClose: function (action) {
					if (action !== MessageBox.Action.OK) {
						return;
					}
					that.api.del(definition.remove.call(that, row)).then(function () {
						that._reload();
						that.toast("deactivated");
					}).catch(function () { /* reported */ });
				}
			});
		}
	});
});
