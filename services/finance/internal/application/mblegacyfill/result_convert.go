package mblegacyfill

// ResultRowsToRowResults converts parsed apply result CSV rows back into RowResults so the
// post-commit recalc and affected-products passes can be re-run (recovery after an interrupt).
// Rows whose (entity, field) is no longer a known writable field keep an empty Spec; they
// never select a recalc target on their own, but their side effects still do.
func ResultRowsToRowResults(items []ResultRow) []RowResult {
	out := make([]RowResult, 0, len(items))
	for _, it := range items {
		row := Row{Line: it.Line, Entity: it.Entity, ID: it.ID, Field: it.Field}
		if spec, ok := Lookup(it.Entity, it.Field); ok {
			row.Spec = spec
		}
		out = append(out, RowResult{
			Row: row, Decision: string(DecisionApply), Result: it.Result,
			BeforeJSON: it.BeforeJSON, AfterJSON: it.AfterJSON, SideEffects: it.SideEffects,
		})
	}
	return out
}
