// Package lookupmaster provides the domain for the mst_lookup_master registry.
package lookupmaster

// LookupMaster represents one registered master table available for MASTER_LOOKUP params.
type LookupMaster struct {
	Code        string
	DisplayName string
	APIPath     string
	CodeField   string
	LabelField  string
	TableName   string
	IsActive    bool
}

// Column represents one fillable column for a master.
type Column struct {
	ID          string // UUID primary key.
	MasterCode  string
	ColumnName  string
	DisplayName string
	DataType    string // "NUMBER" or "TEXT"
	SortOrder   int
}

// UpdateMaster carries the mutable fields for UpdateLookupMaster.
type UpdateMaster struct {
	DisplayName *string
	TableName   *string
	IsActive    *bool
}

// TableColumn is one column from information_schema introspection.
type TableColumn struct {
	ColumnName      string
	DataType        string // "NUMBER" or "TEXT"
	RawType         string // e.g., "numeric", "character varying"
	OrdinalPosition int
}

// MasterOption is one combobox entry (value + label) returned by ListMasterOptions.
type MasterOption struct {
	Value string
	Label string
}
