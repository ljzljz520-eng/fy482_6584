package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"escortcrm/internal/model"
	"escortcrm/internal/service"
	"escortcrm/internal/table"
	"escortcrm/web"
)

func New(customerService *service.Service) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/customers", func(writer http.ResponseWriter, request *http.Request) {
		customers, err := customerService.Customers(model.CustomerFilter{
			Query:               request.URL.Query().Get("q"),
			ServiceCity:         request.URL.Query().Get("city"),
			PatientRelationship: request.URL.Query().Get("relationship"),
		})
		if err != nil {
			writeError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, customers)
	})
	mux.HandleFunc("POST /api/customers", func(writer http.ResponseWriter, request *http.Request) {
		var draft model.CustomerDraft
		if err := decodeJSON(request, &draft); err != nil {
			writeError(writer, err)
			return
		}
		customer, err := customerService.CreateCustomer(draft)
		if err != nil {
			writeError(writer, err)
			return
		}
		writeJSON(writer, http.StatusCreated, customer)
	})
	mux.HandleFunc("GET /api/customers/{id}", func(writer http.ResponseWriter, request *http.Request) {
		customer, err := customerService.Customer(request.PathValue("id"))
		if err != nil {
			writeError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, customer)
	})
	mux.HandleFunc("PUT /api/customers/{id}", func(writer http.ResponseWriter, request *http.Request) {
		var draft model.CustomerDraft
		if err := decodeJSON(request, &draft); err != nil {
			writeError(writer, err)
			return
		}
		customer, err := customerService.UpdateCustomer(request.PathValue("id"), draft)
		if err != nil {
			writeError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, customer)
	})
	mux.HandleFunc("POST /api/imports", func(writer http.ResponseWriter, request *http.Request) {
		var importRequest model.ImportRequest
		if err := decodeJSON(request, &importRequest); err != nil {
			writeError(writer, err)
			return
		}
		result, err := customerService.ImportCustomers(importRequest)
		if err != nil {
			writeError(writer, err)
			return
		}
		writeJSON(writer, http.StatusCreated, result)
	})
	mux.HandleFunc("POST /api/imports/preview", func(writer http.ResponseWriter, request *http.Request) {
		var payload struct {
			Rows []model.CustomerDraft `json:"rows"`
		}
		if err := decodeJSON(request, &payload); err != nil {
			writeError(writer, err)
			return
		}
		preview, err := customerService.PreviewDuplicates(payload.Rows)
		if err != nil {
			writeError(writer, err)
			return
		}
		writeJSON(writer, http.StatusOK, preview)
	})
	mux.Handle("/", web.Handler())
	return mux
}

func decodeJSON(request *http.Request, target any) error {
	defer request.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(nil, request.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return errors.Join(service.ErrInvalid, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.Join(service.ErrInvalid, errors.New("request must contain one JSON value"))
	}
	return nil
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeError(writer http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := "服务器暂时无法处理请求"
	if errors.Is(err, service.ErrInvalid) {
		status = http.StatusBadRequest
		message = strings.TrimPrefix(err.Error(), service.ErrInvalid.Error()+": ")
	}
	if errors.Is(err, table.ErrNotFound) {
		status = http.StatusNotFound
		message = "客户不存在"
	}
	writeJSON(writer, status, map[string]string{"error": message})
}
