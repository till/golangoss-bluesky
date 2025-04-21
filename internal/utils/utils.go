// Package utils provides utility/convenience functions
package utils

import (
	"context"
	"log/slog"
)

// LogError logs an error
func LogError(err error) {
	slog.Error(err.Error(), LogAttr(err))
}

// LogErrorWithContext logs an error with a context
func LogErrorWithContext(ctx context.Context, err error) {
	slog.ErrorContext(ctx, err.Error(), LogAttr(err))
}

// LogAttr returns a slog.Attr for an error
func LogAttr(err error) slog.Attr {
	return slog.Any("error", err)
}
