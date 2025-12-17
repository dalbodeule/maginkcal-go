package ics

import (
	"errors"
	"time"

	"github.com/teambition/rrule-go"

	appLog "epdcal/internal/log"
	"epdcal/internal/model"
)

const (
	defaultMaxOccurrencesPerEvent = 5000
)

// ExpandConfig controls how recurrence expansion is performed.
type ExpandConfig struct {
	// DisplayLocation is the timezone to which all occurrences will be converted.
	// If nil, time.Local is used.
	DisplayLocation *time.Location

	// RangeStart / RangeEnd define the inclusive time window for occurrences.
	RangeStart time.Time
	RangeEnd   time.Time

	// MaxOccurrencesPerEvent is a safety cap to avoid infinite or extremely
	// large expansions. If zero, defaultMaxOccurrencesPerEvent is used.
	MaxOccurrencesPerEvent int
}

// ExpandResult wraps the list of expanded occurrences and optionally
// information about truncation.
type ExpandResult struct {
	Occurrences []model.Occurrence
	// TruncatedEvents records UIDs that hit the MaxOccurrencesPerEvent cap.
	TruncatedEvents []string
}

// ExpandOccurrences takes a list of ParsedEvent (typically for one or more ICS
// sources) and expands them into concrete occurrences within the given time
// range. It handles:
//
//   - Single non-recurring events
//   - RRULE-based recurrence (DAILY/WEEKLY/MONTHLY/YEARLY, etc.)
//   - EXDATE for exception removal
//   - RECURRENCE-ID overrides
//   - All-day semantics
//
// All resulting occurrences are converted into the configured display
// timezone (ExpandConfig.DisplayLocation).
func ExpandOccurrences(events []ParsedEvent, cfg ExpandConfig) (ExpandResult, error) {
	var result ExpandResult

	if cfg.RangeEnd.Before(cfg.RangeStart) {
		return result, errors.New("expand: RangeEnd is before RangeStart")
	}
	if cfg.DisplayLocation == nil {
		cfg.DisplayLocation = time.Local
	}
	if cfg.MaxOccurrencesPerEvent <= 0 {
		cfg.MaxOccurrencesPerEvent = defaultMaxOccurrencesPerEvent
	}

	// Group base events and overrides by UID.
	baseByUID := make(map[string][]ParsedEvent)
	overridesByUID := make(map[string][]ParsedEvent)

	for _, ev := range events {
		if ev.IsOverride && ev.Recurrence != nil {
			overridesByUID[ev.UID] = append(overridesByUID[ev.UID], ev)
		} else {
			baseByUID[ev.UID] = append(baseByUID[ev.UID], ev)
		}
	}

	allOccurrences := make([]model.Occurrence, 0)

	for uid, baseEvents := range baseByUID {
		ov := overridesByUID[uid]
		truncated := false

		for _, ev := range baseEvents {
			occ, hitCap := expandEvent(ev, ov, cfg)
			if hitCap {
				truncated = true
			}
			allOccurrences = append(allOccurrences, occ...)
		}

		if truncated {
			result.TruncatedEvents = append(result.TruncatedEvents, uid)
			appLog.Error("expand: truncated occurrences for UID due to cap",
				errors.New("max occurrences reached"),
				"uid", uid,
				"cap", cfg.MaxOccurrencesPerEvent,
			)
		}
	}

	result.Occurrences = allOccurrences
	return result, nil
}

// expandEvent expands a single ParsedEvent (base event) with its possible
// overrides within the given configuration, returning occurrences and whether
// the cap was hit.
func expandEvent(ev ParsedEvent, overrides []ParsedEvent, cfg ExpandConfig) ([]model.Occurrence, bool) {
	// Single non-recurring event
	if ev.RawRRule == "" {
		return expandSingleEvent(ev, overrides, cfg), false
	}

	// Recurring event via RRULE
	return expandRecurringEvent(ev, overrides, cfg)
}

func expandSingleEvent(ev ParsedEvent, overrides []ParsedEvent, cfg ExpandConfig) []model.Occurrence {
	var out []model.Occurrence

	// Quick range check: if event does not intersect [RangeStart, RangeEnd], skip.
	if !timeRangesOverlap(ev.Start, ev.End, cfg.RangeStart, cfg.RangeEnd) {
		return out
	}

	baseStart := ev.Start
	baseEnd := ev.End

	// Apply any override whose RECURRENCE-ID matches this start.
	if o, ok := findOverrideForStart(ev, overrides, baseStart); ok {
		baseStart = o.Start
		baseEnd = o.End
		ev = o
	}

	out = append(out, makeOccurrence(ev, baseStart, baseEnd, cfg.DisplayLocation))
	return out
}

func expandRecurringEvent(ev ParsedEvent, overrides []ParsedEvent, cfg ExpandConfig) ([]model.Occurrence, bool) {
	out := make([]model.Occurrence, 0)
	hitCap := false

	// Create base rule from RawRRule.
	r, err := rrule.StrToRRule(ev.RawRRule)
	if err != nil {
		appLog.Error("expand: failed to parse RRULE", err, "uid", ev.UID, "rrule", ev.RawRRule)
		return out, false
	}

	// Ensure Dtstart is set to the event's DTSTART.
	r.DTStart(ev.Start)

	// Build a set so we can apply EXDATE.
	var set rrule.Set
	set.RRule(r)

	// Apply EXDATEs.
	for _, ex := range ev.ExDates {
		// Best effort: align EXDATE location with event's start.
		exInLoc := ex.In(ev.Start.Location())
		set.ExDate(exInLoc)
	}

	// Adjust range into the event's original location for Between().
	rangeStart := cfg.RangeStart.In(ev.Start.Location())
	rangeEnd := cfg.RangeEnd.In(ev.Start.Location())

	occTimes := set.Between(rangeStart, rangeEnd, true)

	if len(occTimes) > cfg.MaxOccurrencesPerEvent {
		occTimes = occTimes[:cfg.MaxOccurrencesPerEvent]
		hitCap = true
	}

	for _, occStart := range occTimes {
		var occEnd time.Time
		if ev.AllDay {
			// All-day: treat as [date 00:00, next day 00:00) in event's timezone.
			date := time.Date(occStart.Year(), occStart.Month(), occStart.Day(), 0, 0, 0, 0, occStart.Location())
			occStart = date
			occEnd = date.Add(24 * time.Hour)
		} else {
			// Preserve original duration.
			dur := ev.End.Sub(ev.Start)
			occEnd = occStart.Add(dur)
		}

		baseStart := occStart
		baseEnd := occEnd
		baseEv := ev

		// Apply override if any.
		if o, ok := findOverrideForStart(ev, overrides, occStart); ok {
			baseStart = o.Start
			baseEnd = o.End
			baseEv = o
		}

		out = append(out, makeOccurrence(baseEv, baseStart, baseEnd, cfg.DisplayLocation))
	}

	return out, hitCap
}

// findOverrideForStart finds an override event whose RECURRENCE-ID matches
// the given baseStart (in the base event's timezone) with exact time equality.
func findOverrideForStart(base ParsedEvent, overrides []ParsedEvent, baseStart time.Time) (ParsedEvent, bool) {
	for _, ov := range overrides {
		if ov.Recurrence == nil {
			continue
		}
		// Align recurrence timestamp with base event's location for comparison.
		rid := ov.Recurrence.In(baseStart.Location())
		if rid.Equal(baseStart) {
			return ov, true
		}
	}
	return ParsedEvent{}, false
}

// makeOccurrence converts a (possibly overridden) ParsedEvent + specific
// start/end time into a model.Occurrence normalized into displayLoc.
func makeOccurrence(ev ParsedEvent, start, end time.Time, displayLoc *time.Location) model.Occurrence {
	startLocal := start.In(displayLoc)
	endLocal := end.In(displayLoc)

	occ := model.Occurrence{
		SourceID: ev.Source.ID,
		UID:      ev.UID,
		Summary:  ev.Summary,
		Location: ev.Location,
		AllDay:   ev.AllDay,
		Start:    startLocal,
		End:      endLocal,
	}

	// InstanceKey: use start time in RFC3339 as a stable per-instance key.
	occ.InstanceKey = startLocal.Format(time.RFC3339Nano)

	return occ
}

func timeRangesOverlap(aStart, aEnd, bStart, bEnd time.Time) bool {
	if aEnd.Before(bStart) {
		return false
	}
	if bEnd.Before(aStart) {
		return false
	}
	return true
}
