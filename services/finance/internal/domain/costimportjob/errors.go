package costimportjob

import "errors"

// ErrNotFound is returned when a cost import job does not exist.
var ErrNotFound = errors.New("cost import job not found")
