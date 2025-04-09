package strategy

import "fmt"

// Error sentinel values
var (
	// ErrThresholdExceeded indicates the processing threshold was not met
	ErrThresholdExceeded = fmt.Errorf("processing threshold not met")
)
