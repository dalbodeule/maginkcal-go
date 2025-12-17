package model

import "time"

// Event represents a logical calendar event before recurrence expansion.
// This may be extended later as needed; currently most logic operates on
// ParsedEvent (internal/ics) and Occurrence, but having a central Event
// type is useful for future refactors.
type Event struct {
	SourceID string // calendar source ID (e.g., config ICS ID)
	UID      string // iCalendar UID

	Summary     string
	Description string
	Location    string

	AllDay bool

	// Original start/end in the event's own timezone.
	Start time.Time
	End   time.Time
}

// Occurrence represents a single concrete instance of an event
// (after recurrence expansion and timezone normalization).
type Occurrence struct {
	SourceID string // calendar source ID
	UID      string // iCalendar UID

	// InstanceKey uniquely identifies a single occurrence of a recurring
	// event, typically derived from the local start time.
	InstanceKey string

	Summary     string
	Description string
	Location    string

	AllDay bool

	// Start / End are in the configured display timezone.
	Start time.Time
	End   time.Time
}
