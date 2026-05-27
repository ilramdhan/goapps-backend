package costcalc

// CalculationType selects which cost type (actual / forecast / selling).
type CalculationType string

// CalculationType constants.
const (
	CalcTypeActual   CalculationType = "ACTUAL"
	CalcTypeForecast CalculationType = "FORECAST"
	CalcTypeSelling  CalculationType = "SELLING"
)
