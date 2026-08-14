package httpapi_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
	"time"

	"escortcrm/internal/httpapi"
	"escortcrm/internal/model"
	"escortcrm/internal/service"
	"escortcrm/internal/table"
)

type fixedClock struct {
	value time.Time
}

func (clock fixedClock) Now() time.Time {
	return clock.value
}

type sequenceIDs struct {
	next int
}

func (ids *sequenceIDs) Next(prefix string) string {
	ids.next++
	return fmt.Sprintf("%s%03d", prefix, ids.next)
}

type testApp struct {
	handler http.Handler
	store   *table.Store
}

func newTestApp(t *testing.T) testApp {
	t.Helper()
	store, err := table.Open(t.TempDir())
	if err != nil {
		t.Fatalf("打开本地客户表失败: %v", err)
	}
	clock := fixedClock{value: time.Date(2026, 8, 15, 9, 30, 0, 0, time.FixedZone("CST", 8*60*60))}
	customerService := service.New(store, clock, &sequenceIDs{})
	return testApp{handler: httpapi.New(customerService), store: store}
}

func requestJSON(t *testing.T, handler http.Handler, method, target string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var encoded []byte
	var err error
	if body != nil {
		encoded, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("编码业务请求失败: %v", err)
		}
	}
	request := httptest.NewRequest(method, target, bytes.NewReader(encoded))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func decodeResponse[T any](t *testing.T, response *httptest.ResponseRecorder) T {
	t.Helper()
	var value T
	if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
		t.Fatalf("读取业务响应失败: %v; body=%s", err, response.Body.String())
	}
	return value
}

func sampleCustomer(name, phone, city string) model.CustomerDraft {
	return model.CustomerDraft{
		Name:                name,
		Phone:               phone,
		PatientRelationship: "家属",
		ServiceCity:         city,
		FollowUpAt:          "2026-08-20T01:30:00Z",
		Notes:               "术前检查陪同",
	}
}

func TestCustomerCanBeCreatedChangedFilteredAndOpened(t *testing.T) {
	app := newTestApp(t)
	createdResponse := requestJSON(t, app.handler, http.MethodPost, "/api/customers", sampleCustomer("林女士", "13800001001", "上海"))
	if createdResponse.Code != http.StatusCreated {
		t.Fatalf("创建客户应返回 201，实际为 %d: %s", createdResponse.Code, createdResponse.Body.String())
	}
	created := decodeResponse[model.Customer](t, createdResponse)
	updatedDraft := sampleCustomer("林女士", "13800001001", "杭州")
	updatedDraft.PatientRelationship = "本人"
	updatedDraft.Notes = "改约周四复诊"
	updatedResponse := requestJSON(t, app.handler, http.MethodPut, "/api/customers/"+created.ID, updatedDraft)
	if updatedResponse.Code != http.StatusOK {
		t.Fatalf("修改客户应返回 200，实际为 %d: %s", updatedResponse.Code, updatedResponse.Body.String())
	}
	filteredResponse := requestJSON(t, app.handler, http.MethodGet, "/api/customers?city="+url.QueryEscape("杭州")+"&relationship="+url.QueryEscape("本人"), nil)
	filtered := decodeResponse[[]model.Customer](t, filteredResponse)
	if len(filtered) != 1 || filtered[0].ID != created.ID {
		t.Fatalf("按城市和患者关系筛选应得到已修改客户，实际为 %+v", filtered)
	}
	detailResponse := requestJSON(t, app.handler, http.MethodGet, "/api/customers/"+created.ID, nil)
	detail := decodeResponse[model.Customer](t, detailResponse)
	if detail.Notes != "改约周四复诊" || detail.ServiceCity != "杭州" {
		t.Fatalf("客户详情应展示最新回访信息，实际为 %+v", detail)
	}
}

func TestDuplicatePhonePreviewIncludesSavedAndSubmittedCustomers(t *testing.T) {
	app := newTestApp(t)
	createdResponse := requestJSON(t, app.handler, http.MethodPost, "/api/customers", sampleCustomer("周先生", "139-0000-2001", "南京"))
	if createdResponse.Code != http.StatusCreated {
		t.Fatalf("准备已有客户应返回 201，实际为 %d", createdResponse.Code)
	}
	rows := []model.CustomerDraft{
		sampleCustomer("周先生家属", "13900002001", "南京"),
		sampleCustomer("吴女士", "13700003001", "苏州"),
		sampleCustomer("吴女士家属", "137-0000-3001", "苏州"),
	}
	previewResponse := requestJSON(t, app.handler, http.MethodPost, "/api/imports/preview", map[string]any{"rows": rows})
	if previewResponse.Code != http.StatusOK {
		t.Fatalf("重复手机号预览应返回 200，实际为 %d: %s", previewResponse.Code, previewResponse.Body.String())
	}
	preview := decodeResponse[model.DuplicatePreview](t, previewResponse)
	if len(preview.Duplicates) != 2 {
		t.Fatalf("预览应显示两个重复手机号，实际为 %+v", preview.Duplicates)
	}
	if len(preview.Duplicates[0].ExistingCustomers)+len(preview.Duplicates[1].ExistingCustomers) != 1 {
		t.Fatalf("预览应关联一位已有客户，实际为 %+v", preview.Duplicates)
	}
}

func TestCustomerTablesRemainAvailableAfterReopening(t *testing.T) {
	directory := t.TempDir()
	store, err := table.Open(directory)
	if err != nil {
		t.Fatalf("打开本地客户表失败: %v", err)
	}
	clock := fixedClock{value: time.Date(2026, 8, 15, 9, 30, 0, 0, time.UTC)}
	customerService := service.New(store, clock, &sequenceIDs{})
	created, err := customerService.CreateCustomer(sampleCustomer("陈女士", "13600004001", "成都"))
	if err != nil {
		t.Fatalf("保存客户失败: %v", err)
	}
	reopened, err := table.Open(directory)
	if err != nil {
		t.Fatalf("重新打开本地客户表失败: %v", err)
	}
	loaded, err := reopened.Customer(created.ID)
	if err != nil {
		t.Fatalf("重新打开后读取客户失败: %v", err)
	}
	if !reflect.DeepEqual(loaded, created) {
		t.Fatalf("重新打开后客户内容应保持不变，保存为 %+v，读取为 %+v", created, loaded)
	}
}

func TestRepeatingCompletedImportReturnsOriginalResultAndSingleRecords(t *testing.T) {
	app := newTestApp(t)
	request := model.ImportRequest{
		ImportNumber: "IMPORT-20260815-001",
		Rows:         []model.CustomerDraft{sampleCustomer("赵女士", "13500005001", "北京")},
	}
	firstResponse := requestJSON(t, app.handler, http.MethodPost, "/api/imports", request)
	secondResponse := requestJSON(t, app.handler, http.MethodPost, "/api/imports", request)
	if firstResponse.Code != http.StatusCreated || secondResponse.Code != http.StatusCreated {
		t.Fatalf("两次提交已完成导入都应成功，实际状态为 %d 和 %d", firstResponse.Code, secondResponse.Code)
	}
	first := decodeResponse[model.ImportResult](t, firstResponse)
	second := decodeResponse[model.ImportResult](t, secondResponse)
	if !reflect.DeepEqual(second, first) {
		t.Errorf("第二次提交应返回第一次导入结果，第一次为 %+v，第二次为 %+v", first, second)
	}
	customers, err := app.store.Customers(model.CustomerFilter{})
	if err != nil {
		t.Fatalf("读取客户表失败: %v", err)
	}
	if len(customers) != 1 {
		t.Errorf("连续提交同一导入后客户表应保留一条记录，实际为 %d 条", len(customers))
	}
	operations, err := app.store.Operations()
	if err != nil {
		t.Fatalf("读取操作日志失败: %v", err)
	}
	if len(operations) != 1 {
		t.Errorf("连续提交同一导入后操作日志应保留一条记录，实际为 %d 条", len(operations))
	}
}
