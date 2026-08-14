const state = {
  customers: [],
  selected: null,
  importRows: [],
};

const elements = {
  rows: document.querySelector("#customer-rows"),
  empty: document.querySelector("#empty-state"),
  count: document.querySelector("#customer-count"),
  followups: document.querySelector("#followup-count"),
  status: document.querySelector("#page-status"),
  filters: document.querySelector("#filter-form"),
  customerDialog: document.querySelector("#customer-dialog"),
  customerForm: document.querySelector("#customer-form"),
  customerTitle: document.querySelector("#customer-dialog-title"),
  customerError: document.querySelector("#customer-error"),
  detailDialog: document.querySelector("#detail-dialog"),
  importDialog: document.querySelector("#import-dialog"),
  importNumber: document.querySelector("#import-number"),
  importFile: document.querySelector("#import-file"),
  importSummary: document.querySelector("#import-summary"),
  importError: document.querySelector("#import-error"),
  duplicateResults: document.querySelector("#duplicate-results"),
  previewButton: document.querySelector("#preview-duplicates"),
  submitImport: document.querySelector("#submit-import"),
};

async function api(path, options = {}) {
  const response = await fetch(path, {
    ...options,
    headers: { "Content-Type": "application/json", ...(options.headers || {}) },
  });
  const payload = await response.json();
  if (!response.ok) throw new Error(payload.error || "请求失败");
  return payload;
}

function showStatus(message) {
  elements.status.textContent = message;
}

function formatTime(value) {
  if (!value) return "未安排";
  const date = new Date(value);
  if (Number.isNaN(date.valueOf())) return value;
  return new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "short" }).format(date);
}

function cell(row, value) {
  const item = document.createElement("td");
  if (value instanceof Node) item.append(value);
  else item.textContent = value;
  row.append(item);
}

function renderCustomers() {
  elements.rows.replaceChildren();
  for (const customer of state.customers) {
    const row = document.createElement("tr");
    const person = document.createElement("div");
    person.className = "person";
    const name = document.createElement("strong");
    name.textContent = customer.name;
    const identifier = document.createElement("span");
    identifier.textContent = customer.id;
    person.append(name, identifier);
    cell(row, person);
    cell(row, customer.phone);
    cell(row, customer.patientRelationship);
    cell(row, customer.serviceCity);
    cell(row, formatTime(customer.followUpAt));
    const action = document.createElement("button");
    action.className = "row-action";
    action.type = "button";
    action.textContent = "查看详情";
    action.addEventListener("click", () => openDetail(customer.id));
    cell(row, action);
    elements.rows.append(row);
  }
  elements.empty.hidden = state.customers.length !== 0;
  elements.count.textContent = String(state.customers.length);
  elements.followups.textContent = String(state.customers.filter((customer) => customer.followUpAt).length);
}

async function loadCustomers() {
  const values = new FormData(elements.filters);
  const params = new URLSearchParams();
  for (const [key, value] of values) {
    if (value.trim()) params.set(key, value.trim());
  }
  state.customers = await api(`/api/customers?${params}`);
  renderCustomers();
}

function customerPayload() {
  const values = new FormData(elements.customerForm);
  let followUpAt = values.get("followUpAt");
  if (followUpAt) followUpAt = new Date(followUpAt).toISOString();
  return {
    name: values.get("name"),
    phone: values.get("phone"),
    patientRelationship: values.get("patientRelationship"),
    serviceCity: values.get("serviceCity"),
    followUpAt,
    notes: values.get("notes"),
  };
}

function openCustomerForm(customer = null) {
  elements.customerForm.reset();
  elements.customerError.textContent = "";
  elements.customerForm.elements.id.value = customer?.id || "";
  elements.customerTitle.textContent = customer ? "编辑客户" : "新增客户";
  if (customer) {
    elements.customerForm.elements.name.value = customer.name;
    elements.customerForm.elements.phone.value = customer.phone;
    elements.customerForm.elements.patientRelationship.value = customer.patientRelationship;
    elements.customerForm.elements.serviceCity.value = customer.serviceCity;
    elements.customerForm.elements.notes.value = customer.notes;
    if (customer.followUpAt) elements.customerForm.elements.followUpAt.value = customer.followUpAt.slice(0, 16);
  }
  elements.customerDialog.showModal();
}

async function saveCustomer() {
  if (!elements.customerForm.reportValidity()) return;
  elements.customerError.textContent = "";
  const id = elements.customerForm.elements.id.value;
  try {
    await api(id ? `/api/customers/${id}` : "/api/customers", {
      method: id ? "PUT" : "POST",
      body: JSON.stringify(customerPayload()),
    });
    elements.customerDialog.close();
    await loadCustomers();
    showStatus(id ? "客户档案已更新" : "客户已创建");
  } catch (error) {
    elements.customerError.textContent = error.message;
  }
}

function detailItem(label, value) {
  const wrapper = document.createElement("div");
  const term = document.createElement("dt");
  const detail = document.createElement("dd");
  term.textContent = label;
  detail.textContent = value || "未填写";
  wrapper.append(term, detail);
  return wrapper;
}

async function openDetail(id) {
  state.selected = await api(`/api/customers/${id}`);
  document.querySelector("#detail-name").textContent = state.selected.name;
  const grid = document.querySelector("#detail-grid");
  grid.replaceChildren(
    detailItem("手机号", state.selected.phone),
    detailItem("患者关系", state.selected.patientRelationship),
    detailItem("服务城市", state.selected.serviceCity),
    detailItem("回访时间", formatTime(state.selected.followUpAt)),
    detailItem("创建时间", formatTime(state.selected.createdAt)),
    detailItem("最近更新", formatTime(state.selected.updatedAt)),
  );
  document.querySelector("#detail-notes").textContent = state.selected.notes || "暂无备注";
  elements.detailDialog.showModal();
}

function parseCSV(text) {
  const rows = [];
  let row = [];
  let field = "";
  let quoted = false;
  for (let index = 0; index < text.length; index += 1) {
    const value = text[index];
    if (value === '"' && quoted && text[index + 1] === '"') {
      field += '"';
      index += 1;
    } else if (value === '"') {
      quoted = !quoted;
    } else if (value === "," && !quoted) {
      row.push(field.trim());
      field = "";
    } else if ((value === "\n" || value === "\r") && !quoted) {
      if (value === "\r" && text[index + 1] === "\n") index += 1;
      row.push(field.trim());
      if (row.some(Boolean)) rows.push(row);
      row = [];
      field = "";
    } else {
      field += value;
    }
  }
  row.push(field.trim());
  if (row.some(Boolean)) rows.push(row);
  if (rows.length < 2) throw new Error("客户表中没有可导入的数据");
  const aliases = {
    name: ["name", "姓名"],
    phone: ["phone", "手机号"],
    patientRelationship: ["relationship", "patientRelationship", "患者关系"],
    serviceCity: ["city", "serviceCity", "服务城市"],
    followUpAt: ["followUpAt", "回访时间"],
    notes: ["notes", "备注"],
  };
  const headers = rows[0].map((value) => value.replace(/^\uFEFF/, ""));
  const positions = Object.fromEntries(Object.entries(aliases).map(([key, names]) => [key, headers.findIndex((header) => names.includes(header))]));
  for (const required of ["name", "phone", "patientRelationship", "serviceCity"]) {
    if (positions[required] < 0) throw new Error(`缺少必填表头：${aliases[required].join(" / ")}`);
  }
  return rows.slice(1).map((values) => Object.fromEntries(Object.entries(positions).map(([key, position]) => [key, position < 0 ? "" : values[position] || ""])));
}

async function readImportFile() {
  elements.importError.textContent = "";
  const [file] = elements.importFile.files;
  if (!file) return;
  try {
    state.importRows = parseCSV(await file.text());
    elements.importSummary.textContent = `已读取 ${state.importRows.length} 位客户`;
    elements.previewButton.disabled = false;
    elements.submitImport.disabled = false;
    await previewDuplicates();
  } catch (error) {
    state.importRows = [];
    elements.importSummary.textContent = "客户表读取失败";
    elements.previewButton.disabled = true;
    elements.submitImport.disabled = true;
    elements.importError.textContent = error.message;
  }
}

async function previewDuplicates() {
  elements.importError.textContent = "";
  try {
    const result = await api("/api/imports/preview", { method: "POST", body: JSON.stringify({ rows: state.importRows }) });
    elements.duplicateResults.replaceChildren();
    if (result.duplicates.length === 0) {
      elements.duplicateResults.textContent = "未发现重复手机号";
      return;
    }
    for (const duplicate of result.duplicates) {
      const item = document.createElement("div");
      item.className = "duplicate-item";
      const phone = document.createElement("strong");
      phone.textContent = duplicate.phone;
      const detail = document.createElement("span");
      detail.textContent = `表内 ${duplicate.submittedRows} 条，已有客户 ${duplicate.existingCustomers.length} 位`;
      item.append(phone, detail);
      elements.duplicateResults.append(item);
    }
  } catch (error) {
    elements.importError.textContent = error.message;
  }
}

function openImport() {
  state.importRows = [];
  elements.importNumber.value = "";
  elements.importFile.value = "";
  elements.importError.textContent = "";
  elements.importSummary.textContent = "尚未选择客户表";
  elements.duplicateResults.textContent = "暂无预览结果";
  elements.previewButton.disabled = true;
  elements.submitImport.disabled = true;
  elements.importDialog.showModal();
}

async function submitImport() {
  if (!elements.importNumber.value.trim()) {
    elements.importError.textContent = "请填写导入编号";
    return;
  }
  try {
    const result = await api("/api/imports", {
      method: "POST",
      body: JSON.stringify({ importNumber: elements.importNumber.value.trim(), rows: state.importRows }),
    });
    elements.importDialog.close();
    await loadCustomers();
    showStatus(`导入完成：${result.imported} 位客户`);
  } catch (error) {
    elements.importError.textContent = error.message;
  }
}

elements.filters.addEventListener("submit", (event) => { event.preventDefault(); loadCustomers().catch((error) => showStatus(error.message)); });
document.querySelector("#reset-filter").addEventListener("click", () => { elements.filters.reset(); loadCustomers().catch((error) => showStatus(error.message)); });
document.querySelector("#open-create").addEventListener("click", () => openCustomerForm());
document.querySelector("#save-customer").addEventListener("click", saveCustomer);
document.querySelector("#close-detail").addEventListener("click", () => elements.detailDialog.close());
document.querySelector("#edit-customer").addEventListener("click", () => { elements.detailDialog.close(); openCustomerForm(state.selected); });
document.querySelector("#open-import").addEventListener("click", openImport);
document.querySelector("#close-import").addEventListener("click", () => elements.importDialog.close());
document.querySelector("#cancel-import").addEventListener("click", () => elements.importDialog.close());
elements.importFile.addEventListener("change", readImportFile);
elements.previewButton.addEventListener("click", previewDuplicates);
elements.submitImport.addEventListener("click", submitImport);

loadCustomers().catch((error) => showStatus(error.message));
