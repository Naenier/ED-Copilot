// Package freshness describes the age and coverage of observations independently
// of the outcome of a later refresh attempt.
package freshness

import "time"

type State string

const (
	Unknown State = "unknown"
	Fresh   State = "fresh"
	Stale   State = "stale"
	Undated State = "undated"
)

type Coverage string

const (
	Complete Coverage = "complete"
	Partial  Coverage = "partial"
	Empty    Coverage = "empty"
)

type Reason string

const (
	Offline     Reason = "offline"
	RateLimited Reason = "rate-limited"
	NoMatch     Reason = "no-match"
	Incomplete  Reason = "incomplete"
	Unavailable Reason = "unavailable"
)

type Observation struct {
	State      State    `json:"state"`
	Coverage   Coverage `json:"coverage,omitempty"`
	ObservedAt string   `json:"observedAt,omitempty"`
	FetchedAt  string   `json:"fetchedAt,omitempty"`
}

func Describe(now time.Time, maxAge time.Duration, known bool, coverage Coverage, observed, fetched time.Time) Observation {
	quality := Observation{State: Unknown, Coverage: coverage}
	if !observed.IsZero() {
		quality.ObservedAt = observed.UTC().Format(time.RFC3339Nano)
	}
	if !fetched.IsZero() {
		quality.FetchedAt = fetched.UTC().Format(time.RFC3339Nano)
	}
	if !known {
		return quality
	}
	stamp := observed
	if stamp.IsZero() {
		stamp = fetched
	}
	switch {
	case stamp.IsZero(), stamp.After(now):
		quality.State = Undated
	case now.Sub(stamp) > maxAge:
		quality.State = Stale
	default:
		quality.State = Fresh
	}
	return quality
}
