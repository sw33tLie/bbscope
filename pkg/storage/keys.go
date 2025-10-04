package storage

import (
	"fmt"
)

func identityKey(tnorm, category string, inScope bool) string {
	if tnorm == "" || category == "" {
		return ""
	}
	return fmt.Sprintf("%s|%s|%d", tnorm, category, boolToInt(inScope))
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
