package ratelimiting

import (
	"time"
)

type clientEntry struct {
	sessions      []session
	lastUpdatedAt time.Time
}

func (c clientEntry) isBanned() bool {
	var timeZero time.Time
	for _, s := range c.sessions {
		if s.bannedAt != timeZero {
			return true
		}
	}
	return false
}

func newClientEntry(t time.Time, sessionConfigs []sessionConfig) clientEntry {
	logger.Debug("", "Inside newClientEntry")
	s := []session{}
	for _, c := range sessionConfigs {
		s = append(s, session{
			startedAt: t,
			config:    c,
		})
	}
	return clientEntry{
		sessions:      s,
		lastUpdatedAt: t,
	}
}

func newUpdatedEntry(existingEntry clientEntry, t time.Time) clientEntry {
	updatedEntry := clientEntry{
		sessions:      make([]session, 0, len(existingEntry.sessions)),
		lastUpdatedAt: t,
	}
	for _, s := range existingEntry.sessions {
		updatedSession := session{
			bannedAt:  s.bannedAt,
			startedAt: s.startedAt,
			config:    s.config,
		}
		currentSessionLength := t.Sub(s.startedAt)
		if currentSessionLength >= s.config.sessionDuration {
			if s.requestCount > s.config.maxRequests {
				// Establish or extend the ban.
				updatedSession.bannedAt = t
			}
			updatedSession.requestCount = 1
			updatedSession.startedAt = t
		} else {
			updatedSession.requestCount = s.requestCount + 1
		}
		updatedEntry.sessions = append(updatedEntry.sessions, updatedSession)
	}
	logger.Debug("newUpdatedEntry", "updated client entry: %+v", updatedEntry)
	return updatedEntry
}
