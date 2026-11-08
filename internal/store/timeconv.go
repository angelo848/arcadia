package store

import "time"

// TimeToMillis converts an instant to Unix milliseconds, the storage format of
// every *_at column (ADR 0009). The zone of t is irrelevant.
func TimeToMillis(t time.Time) int64 {
	return t.UnixMilli()
}

// MillisToTime converts Unix milliseconds read from a *_at column back to an
// instant in UTC.
func MillisToTime(ms int64) time.Time {
	return time.UnixMilli(ms).UTC()
}
