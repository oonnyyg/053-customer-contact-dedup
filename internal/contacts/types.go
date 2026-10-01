package contacts

// Record is a single imported contact record.
type Record struct {
	Source  string `json:"source"`
	ID      string `json:"id"`
	Name    string `json:"name"`
	Company string `json:"company,omitempty"`
	Email   string `json:"email,omitempty"`
	Phone   string `json:"phone,omitempty"`
}

// normalized holds canonical comparison values. Empty values mean "absent".
type normalized struct {
	name    string
	company string
	email   string
	phone   string
}

// Ref identifies a record by its source and source-local id.
type Ref struct {
	Source string `json:"source"`
	ID     string `json:"id"`
}

// FieldChoice selects which source record supplies a merged field value.
type FieldChoice struct {
	Source string `json:"source"`
	ID     string `json:"id"`
}

// FieldSource records the provenance of one merged output field.
type FieldSource struct {
	Field  string `json:"field"`
	Source string `json:"source"`
	ID     string `json:"id"`
}

// Archive is a confirmed merged contact profile.
type Archive struct {
	ArchiveID  string        `json:"archive_id"`
	Name       string        `json:"name,omitempty"`
	Company    string        `json:"company,omitempty"`
	Email      string        `json:"email,omitempty"`
	Phone      string        `json:"phone,omitempty"`
	Sources    []Ref         `json:"sources"`
	Provenance []FieldSource `json:"provenance"`
}
