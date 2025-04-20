package utils

import (
	"context"
	"log/slog"
)

func LogError(err error) {
	slog.Error(err.Error(), LogAttr(err))
}

func LogErrorWithContext(ctx context.Context, err error) {
	slog.ErrorContext(ctx, err.Error(), LogAttr(err))
}

func LogAttr(err error) slog.Attr {
	return slog.Any("error", err)
}
