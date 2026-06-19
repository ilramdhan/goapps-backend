// Package lookupmaster provides the domain for the mst_lookup_master registry.
package lookupmaster

// LookupMaster represents one registered master table available for MASTER_LOOKUP params.
type LookupMaster struct {
	Code        string
	DisplayName string
	APIPath     string
	CodeField   string
	LabelField  string
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
