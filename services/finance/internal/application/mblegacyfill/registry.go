// Package mblegacyfill fills MB head/spin columns from the reviewed Oracle legacy proposal CSV.
package mblegacyfill

import "strings"

// Entity is the MB table family a proposal row targets.
type Entity string

// Entity values.
const (
	EntityHead Entity = "head"
	EntitySpin Entity = "spin"
)

// Class is the write class of a field (design section 3).
type Class string

// Field classes.
const (
	ClassL Class = "L" // legacy wins, plain column
	ClassD Class = "D" // derived/cascading, uniqueness-checked
	ClassS Class = "S" // shade master related
	ClassX Class = "X" // never written
)

// Kind is the value kind of a column.
type Kind string

// Value kinds.
const (
	KindText Kind = "text"
	KindNum  Kind = "num"
	KindInt  Kind = "int"
	KindBool Kind = "bool"
)

// FieldSpec describes one writable (entity, field).
type FieldSpec struct {
	Entity Entity
	Field  string
	Column string
	Kind   Kind
	Class  Class
}

type regKey struct {
	entity Entity
	field  string
}

type regDef struct {
	field, headCol, spinCol string
	kind                    Kind
	class                   Class
}

var registryDefs = []regDef{
	{"status", "mbh_status", "mbs_status", KindText, ClassL},
	{"check_status", "mbh_check_status", "", KindText, ClassL},
	{"ldr_prsn", "mbh_ldr_prsn", "mbs_ldr_prsn", KindNum, ClassL},
	{"final", "mbh_final_product", "mbs_final_product", KindText, ClassL},
	{"lesture", "mbh_lesture", "mbs_lesture", KindText, ClassL},
	{"d_f", "mbh_d_f", "mbs_d_f", KindText, ClassL},
	{"notes", "notes", "notes", KindText, ClassL},
	{"no_of_process", "mbh_no_of_process", "", KindText, ClassL},
	{"lusture_code", "mbh_lusture_code", "", KindText, ClassL},
	{"spg", "mbh_mb_spg_orion", "mbs_mb_spg_orion", KindText, ClassL},
	{"cc", "", "mbs_cc", KindText, ClassL},
	{"mb_costing", "mbh_mb_costing", "mbs_mb_costing", KindText, ClassD},
	{"mgt_name", "mbh_mgt_name", "mbs_mgt_name", KindText, ClassD},
	{"orion", "mbh_orion_item_code", "mbs_orion_item_code", KindText, ClassD},
	{"code", "mbh_code", "", KindText, ClassD},
	{"vs", "mbh_vs_number", "mbs_vs_number", KindText, ClassD},
	{"dev", "mbh_dev_code", "", KindText, ClassD},
	{"den", "mbh_denier", "mbs_denier", KindNum, ClassD},
	{"fil", "mbh_filament", "mbs_filament", KindInt, ClassD},
	{"doz", "mbh_dozing", "mbs_dozing", KindNum, ClassD},
	{"run_ldr", "mbh_run_ldr_pct", "mbs_run_ldr_pct", KindNum, ClassD},
	{"shade_code", "mbh_shade_code", "mbs_shade_code", KindText, ClassS},
	{"shade_name", "mbh_shade_name", "mbs_shade_name", KindText, ClassS},
}

var registry = buildRegistry()

func buildRegistry() map[regKey]FieldSpec {
	m := make(map[regKey]FieldSpec, len(registryDefs)*2)
	for _, d := range registryDefs {
		if d.headCol != "" {
			m[regKey{EntityHead, d.field}] = FieldSpec{EntityHead, d.field, d.headCol, d.kind, d.class}
		}
		if d.spinCol != "" {
			m[regKey{EntitySpin, d.field}] = FieldSpec{EntitySpin, d.field, d.spinCol, d.kind, d.class}
		}
	}
	return m
}

// Lookup returns the spec for (entity, field). ok=false means the pair is
// unknown or class X and must be REJECTed (entry_status, is_locked, param_*, fg*, ...).
func Lookup(entity Entity, field string) (FieldSpec, bool) {
	s, ok := registry[regKey{entity, strings.ToLower(strings.TrimSpace(field))}]
	return s, ok && s.Class != ClassX
}

// ValidEntity reports whether e is head or spin.
func ValidEntity(e Entity) bool { return e == EntityHead || e == EntitySpin }

// spinExtraCols are spin columns written outside the registry (ACTUAL marker and
// head->spin cascade) that a revert is also allowed to restore.
var spinExtraCols = map[string]Kind{
	"mbs_ldr_type": KindText, "mbs_ldr_is_actual": KindBool,
	"mbs_lusture_code": KindText, "mbs_cross_section": KindText,
}

// ColumnKind returns the kind of a column the tool may have written for the entity
// (registry columns plus the spin extras). ok=false means the column is not
// allow-listed (revert must never build SQL from it).
func ColumnKind(entity Entity, col string) (Kind, bool) {
	for _, d := range registryDefs {
		c := d.headCol
		if entity == EntitySpin {
			c = d.spinCol
		}
		if c != "" && c == col {
			return d.kind, true
		}
	}
	if entity == EntitySpin {
		k, ok := spinExtraCols[col]
		return k, ok
	}
	return "", false
}

// CostProductKind returns the kind of a cost_product_master column touched by the head cascade.
func CostProductKind(col string) (Kind, bool) {
	switch col {
	case "cpm_product_name", "cpm_shade_code", "cpm_shade_name":
		return KindText, true
	}
	return "", false
}
