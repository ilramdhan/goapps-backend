package validation

import (
	"strconv"
	"time"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// V10 checks demand freshness (now − ceib_demand_loaded_at <= 24h, Q11) and
// that the live read-only probe found 0 posted heads for the period (design
// §7 V-10; error, batch). A missing timestamp or probe fails closed.
func V10() Validator {
	return validatorFunc{code: CodeV10, fn: func(in *Input) []Finding {
		var out []Finding
		maxAge := in.DemandMaxAge
		if maxAge <= 0 {
			maxAge = DefaultDemandMaxAge
		}
		switch {
		case in.DemandLoadedAt == nil:
			out = append(out, batchFinding(CodeV10, "demand has never been loaded"))
		case in.Now.Sub(*in.DemandLoadedAt) > maxAge:
			out = append(out, batchFinding(CodeV10, "demand loaded at "+in.DemandLoadedAt.UTC().Format(time.RFC3339)+
				" is older than "+maxAge.String()+"; reload demand"))
		}
		switch {
		case in.PostedHeads == nil:
			out = append(out, batchFinding(CodeV10, "posted-head probe was not run"))
		case *in.PostedHeads > 0:
			out = append(out, batchFinding(CodeV10, strconv.FormatInt(*in.PostedHeads, 10)+
				" ADJ head(s) of "+in.Period+" are posted"))
		}
		return out
	}}
}

func batchFinding(code erpintegration.IssueCode, msg string) Finding {
	return Finding{Code: code, Severity: SeverityError, Scope: ScopeBatch, Message: msg}
}
