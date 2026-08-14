package service

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"escortcrm/internal/model"
	"escortcrm/internal/table"
)

var ErrInvalid = errors.New("invalid request")

type Clock interface {
	Now() time.Time
}

type IDGenerator interface {
	Next(prefix string) string
}

type SystemClock struct{}

func (SystemClock) Now() time.Time {
	return time.Now()
}

type RandomIDs struct{}

func (RandomIDs) Next(prefix string) string {
	value := make([]byte, 10)
	if _, err := rand.Read(value); err != nil {
		return fmt.Sprintf("%s%d", prefix, time.Now().UnixNano())
	}
	return prefix + hex.EncodeToString(value)
}

type Service struct {
	store *table.Store
	clock Clock
	ids   IDGenerator
}

func New(store *table.Store, clock Clock, ids IDGenerator) *Service {
	return &Service{store: store, clock: clock, ids: ids}
}

func (s *Service) CreateCustomer(draft model.CustomerDraft) (model.Customer, error) {
	if err := validateDraft(draft); err != nil {
		return model.Customer{}, err
	}
	now := s.clock.Now().UTC().Format(time.RFC3339)
	customer := customerFromDraft(s.ids.Next("cus_"), draft, now)
	operation := model.Operation{ID: s.ids.Next("op_"), Action: "customer.created", CustomerID: customer.ID, OccurredAt: now}
	if err := s.store.CreateCustomer(customer, operation); err != nil {
		return model.Customer{}, err
	}
	return customer, nil
}

func (s *Service) UpdateCustomer(id string, draft model.CustomerDraft) (model.Customer, error) {
	if err := validateDraft(draft); err != nil {
		return model.Customer{}, err
	}
	existing, err := s.store.Customer(id)
	if err != nil {
		return model.Customer{}, err
	}
	now := s.clock.Now().UTC().Format(time.RFC3339)
	updated := customerFromDraft(existing.ID, draft, existing.CreatedAt)
	updated.UpdatedAt = now
	operation := model.Operation{ID: s.ids.Next("op_"), Action: "customer.updated", CustomerID: updated.ID, OccurredAt: now}
	if err := s.store.UpdateCustomer(updated, operation); err != nil {
		return model.Customer{}, err
	}
	return updated, nil
}

func (s *Service) ImportCustomers(request model.ImportRequest) (model.ImportResult, error) {
	request.ImportNumber = strings.TrimSpace(request.ImportNumber)
	if request.ImportNumber == "" || len(request.Rows) == 0 {
		return model.ImportResult{}, fmt.Errorf("%w: import number and rows are required", ErrInvalid)
	}
	if existing, ok, err := s.store.CompletedImport(request.ImportNumber); err != nil {
		return model.ImportResult{}, err
	} else if ok {
		return existing, nil
	}
	now := s.clock.Now().UTC().Format(time.RFC3339)
	customers := make([]model.Customer, 0, len(request.Rows))
	operations := make([]model.Operation, 0, len(request.Rows))
	customerIDs := make([]string, 0, len(request.Rows))
	for _, draft := range request.Rows {
		if err := validateDraft(draft); err != nil {
			return model.ImportResult{}, err
		}
		customer := customerFromDraft(s.ids.Next("cus_"), draft, now)
		customers = append(customers, customer)
		customerIDs = append(customerIDs, customer.ID)
		operations = append(operations, model.Operation{ID: s.ids.Next("op_"), ImportID: request.ImportNumber, Action: "customer.imported", CustomerID: customer.ID, OccurredAt: now})
	}
	result := model.ImportResult{ImportNumber: request.ImportNumber, CustomerIDs: customerIDs, Imported: len(customers), CompletedAt: now}
	if err := s.store.RecordImport(customers, operations, result); err != nil {
		return model.ImportResult{}, err
	}
	return result, nil
}

func (s *Service) Customer(id string) (model.Customer, error) {
	return s.store.Customer(id)
}

func (s *Service) Customers(filter model.CustomerFilter) ([]model.Customer, error) {
	return s.store.Customers(filter)
}

func (s *Service) PreviewDuplicates(rows []model.CustomerDraft) (model.DuplicatePreview, error) {
	if len(rows) == 0 {
		return model.DuplicatePreview{}, fmt.Errorf("%w: rows are required", ErrInvalid)
	}
	existing, err := s.store.Customers(model.CustomerFilter{})
	if err != nil {
		return model.DuplicatePreview{}, err
	}
	existingByPhone := make(map[string][]model.Customer)
	for _, customer := range existing {
		key := phoneKey(customer.Phone)
		if key != "" {
			existingByPhone[key] = append(existingByPhone[key], customer)
		}
	}
	counts := make(map[string]int)
	displayPhone := make(map[string]string)
	for _, row := range rows {
		key := phoneKey(row.Phone)
		if key == "" {
			continue
		}
		counts[key]++
		if displayPhone[key] == "" {
			displayPhone[key] = strings.TrimSpace(row.Phone)
		}
	}
	keys := make([]string, 0, len(counts))
	for key, count := range counts {
		if count > 1 || len(existingByPhone[key]) > 0 {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	duplicates := make([]model.DuplicatePhone, 0, len(keys))
	for _, key := range keys {
		duplicates = append(duplicates, model.DuplicatePhone{Phone: displayPhone[key], SubmittedRows: counts[key], ExistingCustomers: existingByPhone[key]})
	}
	return model.DuplicatePreview{Duplicates: duplicates}, nil
}

func customerFromDraft(id string, draft model.CustomerDraft, createdAt string) model.Customer {
	return model.Customer{
		ID:                  id,
		Name:                strings.TrimSpace(draft.Name),
		Phone:               strings.TrimSpace(draft.Phone),
		PatientRelationship: strings.TrimSpace(draft.PatientRelationship),
		ServiceCity:         strings.TrimSpace(draft.ServiceCity),
		FollowUpAt:          strings.TrimSpace(draft.FollowUpAt),
		Notes:               strings.TrimSpace(draft.Notes),
		CreatedAt:           createdAt,
		UpdatedAt:           createdAt,
	}
}

func validateDraft(draft model.CustomerDraft) error {
	if strings.TrimSpace(draft.Name) == "" || strings.TrimSpace(draft.Phone) == "" || strings.TrimSpace(draft.PatientRelationship) == "" || strings.TrimSpace(draft.ServiceCity) == "" {
		return fmt.Errorf("%w: name, phone, patient relationship and service city are required", ErrInvalid)
	}
	if draft.FollowUpAt != "" {
		if _, err := time.Parse(time.RFC3339, draft.FollowUpAt); err != nil {
			return fmt.Errorf("%w: follow-up time must use RFC3339", ErrInvalid)
		}
	}
	return nil
}

func phoneKey(phone string) string {
	var builder strings.Builder
	for _, value := range phone {
		if unicode.IsDigit(value) {
			builder.WriteRune(value)
		}
	}
	if builder.Len() > 0 {
		return builder.String()
	}
	return strings.ToLower(strings.TrimSpace(phone))
}
