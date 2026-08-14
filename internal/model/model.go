package model

type Customer struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	Phone               string `json:"phone"`
	PatientRelationship string `json:"patientRelationship"`
	ServiceCity         string `json:"serviceCity"`
	FollowUpAt          string `json:"followUpAt"`
	Notes               string `json:"notes"`
	CreatedAt           string `json:"createdAt"`
	UpdatedAt           string `json:"updatedAt"`
}

type CustomerDraft struct {
	Name                string `json:"name"`
	Phone               string `json:"phone"`
	PatientRelationship string `json:"patientRelationship"`
	ServiceCity         string `json:"serviceCity"`
	FollowUpAt          string `json:"followUpAt"`
	Notes               string `json:"notes"`
}

type CustomerFilter struct {
	Query               string
	ServiceCity         string
	PatientRelationship string
}

type Operation struct {
	ID         string `json:"id"`
	ImportID   string `json:"importId"`
	Action     string `json:"action"`
	CustomerID string `json:"customerId"`
	OccurredAt string `json:"occurredAt"`
}

type ImportRequest struct {
	ImportNumber string          `json:"importNumber"`
	Rows         []CustomerDraft `json:"rows"`
}

type ImportResult struct {
	ImportNumber string   `json:"importNumber"`
	CustomerIDs  []string `json:"customerIds"`
	Imported     int      `json:"imported"`
	CompletedAt  string   `json:"completedAt"`
}

type DuplicatePhone struct {
	Phone             string     `json:"phone"`
	SubmittedRows     int        `json:"submittedRows"`
	ExistingCustomers []Customer `json:"existingCustomers"`
}

type DuplicatePreview struct {
	Duplicates []DuplicatePhone `json:"duplicates"`
}
