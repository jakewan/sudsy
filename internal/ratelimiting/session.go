package ratelimiting

import "time"

type session struct {
	requestCount int64
	config       sessionConfig
	bannedAt     time.Time
	startedAt    time.Time
}
