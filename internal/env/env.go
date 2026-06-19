// Package env reads configuration from environment variables. It is the single place
// env parsing lives (DRY), shared by the resolver CLI and the signing proxy.
package env

import (
	"os"
	"time"
)

// Or returns the value of key, or def when unset/empty.
func Or(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// Bool reports whether key is set to a truthy value (1/true/yes/on, case-insensitive).
func Bool(key string) bool {
	switch v := os.Getenv(key); v {
	case "1", "true", "TRUE", "True", "yes", "on":
		return true
	default:
		return false
	}
}

// Duration parses key as a Go duration, falling back to def on unset/invalid.
func Duration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
