package rsh

import (
	"log/slog"
	"strconv"
)

func parseUint16(s string) uint16 {
	u, err := strconv.ParseUint(s, 10, 16)
	if err != nil {
		slog.Info("Error parsing uint:", slog.Any("error", err))
		return 0
	}

	return uint16(u)
}
