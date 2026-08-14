package table

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"escortcrm/internal/model"
)

var ErrNotFound = errors.New("record not found")

var customerHeader = []string{"id", "name", "phone", "patient_relationship", "service_city", "follow_up_at", "notes", "created_at", "updated_at"}
var operationHeader = []string{"id", "import_number", "action", "customer_id", "occurred_at"}
var importHeader = []string{"import_number", "customer_ids", "imported", "completed_at"}

type Store struct {
	dir string
	mu  sync.Mutex
}

func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	s := &Store{dir: dir}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, table := range []struct {
		name   string
		header []string
	}{
		{name: "customers.csv", header: customerHeader},
		{name: "operations.csv", header: operationHeader},
		{name: "imports.csv", header: importHeader},
	} {
		if err := s.ensureTableLocked(table.name, table.header); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *Store) CreateCustomer(customer model.Customer, operation model.Operation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.appendRowsLocked("customers.csv", [][]string{customerRow(customer)}); err != nil {
		return err
	}
	return s.appendRowsLocked("operations.csv", [][]string{operationRow(operation)})
}

func (s *Store) UpdateCustomer(customer model.Customer, operation model.Operation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.readRowsLocked("customers.csv", customerHeader)
	if err != nil {
		return err
	}
	found := false
	for i, row := range rows {
		if row[0] == customer.ID {
			rows[i] = customerRow(customer)
			found = true
			break
		}
	}
	if !found {
		return ErrNotFound
	}
	if err := s.replaceRowsLocked("customers.csv", customerHeader, rows); err != nil {
		return err
	}
	return s.appendRowsLocked("operations.csv", [][]string{operationRow(operation)})
}

func (s *Store) RecordImport(customers []model.Customer, operations []model.Operation, result model.ImportResult) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	customerRows := make([][]string, 0, len(customers))
	for _, customer := range customers {
		customerRows = append(customerRows, customerRow(customer))
	}
	operationRows := make([][]string, 0, len(operations))
	for _, operation := range operations {
		operationRows = append(operationRows, operationRow(operation))
	}
	if err := s.appendRowsLocked("customers.csv", customerRows); err != nil {
		return err
	}
	if err := s.appendRowsLocked("operations.csv", operationRows); err != nil {
		return err
	}
	encodedIDs, err := json.Marshal(result.CustomerIDs)
	if err != nil {
		return fmt.Errorf("encode import result: %w", err)
	}
	return s.appendRowsLocked("imports.csv", [][]string{{result.ImportNumber, string(encodedIDs), strconv.Itoa(result.Imported), result.CompletedAt}})
}

func (s *Store) CompletedImport(importNumber string) (model.ImportResult, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.readRowsLocked("imports.csv", importHeader)
	if err != nil {
		return model.ImportResult{}, false, err
	}
	for _, row := range rows {
		if row[0] != importNumber {
			continue
		}
		var customerIDs []string
		if err := json.Unmarshal([]byte(row[1]), &customerIDs); err != nil {
			return model.ImportResult{}, false, fmt.Errorf("decode import result: %w", err)
		}
		imported, err := strconv.Atoi(row[2])
		if err != nil {
			return model.ImportResult{}, false, fmt.Errorf("decode imported count: %w", err)
		}
		return model.ImportResult{ImportNumber: row[0], CustomerIDs: customerIDs, Imported: imported, CompletedAt: row[3]}, true, nil
	}
	return model.ImportResult{}, false, nil
}

func (s *Store) Customer(id string) (model.Customer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.readRowsLocked("customers.csv", customerHeader)
	if err != nil {
		return model.Customer{}, err
	}
	for _, row := range rows {
		if row[0] == id {
			return customerFromRow(row), nil
		}
	}
	return model.Customer{}, ErrNotFound
}

func (s *Store) Customers(filter model.CustomerFilter) ([]model.Customer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.readRowsLocked("customers.csv", customerHeader)
	if err != nil {
		return nil, err
	}
	query := strings.ToLower(strings.TrimSpace(filter.Query))
	city := strings.ToLower(strings.TrimSpace(filter.ServiceCity))
	relationship := strings.ToLower(strings.TrimSpace(filter.PatientRelationship))
	customers := make([]model.Customer, 0, len(rows))
	for _, row := range rows {
		customer := customerFromRow(row)
		if city != "" && strings.ToLower(customer.ServiceCity) != city {
			continue
		}
		if relationship != "" && strings.ToLower(customer.PatientRelationship) != relationship {
			continue
		}
		searchable := strings.ToLower(customer.Name + " " + customer.Phone + " " + customer.Notes)
		if query != "" && !strings.Contains(searchable, query) {
			continue
		}
		customers = append(customers, customer)
	}
	sort.SliceStable(customers, func(i, j int) bool {
		return customers[i].CreatedAt > customers[j].CreatedAt
	})
	return customers, nil
}

func (s *Store) Operations() ([]model.Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.readRowsLocked("operations.csv", operationHeader)
	if err != nil {
		return nil, err
	}
	operations := make([]model.Operation, 0, len(rows))
	for _, row := range rows {
		operations = append(operations, model.Operation{ID: row[0], ImportID: row[1], Action: row[2], CustomerID: row[3], OccurredAt: row[4]})
	}
	return operations, nil
}

func (s *Store) ensureTableLocked(name string, header []string) error {
	path := filepath.Join(s.dir, name)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, os.ErrExist) {
		rows, readErr := s.readRowsLocked(name, header)
		if readErr != nil {
			return readErr
		}
		_ = rows
		return nil
	}
	if err != nil {
		return fmt.Errorf("create %s: %w", name, err)
	}
	writer := csv.NewWriter(file)
	writeErr := writer.Write(header)
	writer.Flush()
	if writeErr == nil {
		writeErr = writer.Error()
	}
	closeErr := file.Close()
	if writeErr != nil {
		return fmt.Errorf("initialize %s: %w", name, writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close %s: %w", name, closeErr)
	}
	return nil
}

func (s *Store) readRowsLocked(name string, header []string) ([][]string, error) {
	file, err := os.Open(filepath.Join(s.dir, name))
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", name, err)
	}
	defer file.Close()
	reader := csv.NewReader(file)
	actualHeader, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("read %s header: %w", name, err)
	}
	if strings.Join(actualHeader, "\x00") != strings.Join(header, "\x00") {
		return nil, fmt.Errorf("unexpected %s header", name)
	}
	rows, err := reader.ReadAll()
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	for _, row := range rows {
		if len(row) != len(header) {
			return nil, fmt.Errorf("unexpected %s row width", name)
		}
	}
	return rows, nil
}

func (s *Store) appendRowsLocked(name string, rows [][]string) error {
	if len(rows) == 0 {
		return nil
	}
	file, err := os.OpenFile(filepath.Join(s.dir, name), os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open %s for append: %w", name, err)
	}
	writer := csv.NewWriter(file)
	for _, row := range rows {
		if err := writer.Write(row); err != nil {
			file.Close()
			return fmt.Errorf("append %s: %w", name, err)
		}
	}
	writer.Flush()
	writeErr := writer.Error()
	closeErr := file.Close()
	if writeErr != nil {
		return fmt.Errorf("append %s: %w", name, writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close %s: %w", name, closeErr)
	}
	return nil
}

func (s *Store) replaceRowsLocked(name string, header []string, rows [][]string) error {
	temporary, err := os.CreateTemp(s.dir, name+"-*")
	if err != nil {
		return fmt.Errorf("create replacement for %s: %w", name, err)
	}
	temporaryName := temporary.Name()
	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = os.Remove(temporaryName)
		}
	}()
	writer := csv.NewWriter(temporary)
	if err := writer.Write(header); err == nil {
		err = writer.WriteAll(rows)
	}
	writer.Flush()
	if err == nil {
		err = writer.Error()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("write replacement for %s: %w", name, err)
	}
	if err := os.Rename(temporaryName, filepath.Join(s.dir, name)); err != nil {
		return fmt.Errorf("replace %s: %w", name, err)
	}
	removeTemporary = false
	return nil
}

func customerRow(customer model.Customer) []string {
	return []string{customer.ID, customer.Name, customer.Phone, customer.PatientRelationship, customer.ServiceCity, customer.FollowUpAt, customer.Notes, customer.CreatedAt, customer.UpdatedAt}
}

func customerFromRow(row []string) model.Customer {
	return model.Customer{ID: row[0], Name: row[1], Phone: row[2], PatientRelationship: row[3], ServiceCity: row[4], FollowUpAt: row[5], Notes: row[6], CreatedAt: row[7], UpdatedAt: row[8]}
}

func operationRow(operation model.Operation) []string {
	return []string{operation.ID, operation.ImportID, operation.Action, operation.CustomerID, operation.OccurredAt}
}
