package ics

import (
	"bytes"
	"errors"
	"strconv"
	"strings"
	"time"

	ical "github.com/arran4/golang-ical"

	appLog "epdcal/internal/log"
)

// ParsedEvent is the normalized representation of a VEVENT as produced
// by the ICS parser. Recurrence expansion will operate on this type.
type ParsedEvent struct {
	Source Source

	UID string
	Seq int

	Summary     string
	Description string
	Location    string

	Start   time.Time
	End     time.Time
	AllDay  bool
	StartTZ string
	EndTZ   string

	RawRRule   string
	ExDates    []time.Time
	Recurrence *time.Time // RECURRENCE-ID (if present) in event's own timezone
	IsOverride bool       // true if this VEVENT is an override for a recurring instance
}

// ParseICS parses a single ICS payload into a list of ParsedEvent.
//
//   - It relies on the underlying library's VTIMEZONE/TZID handling to
//     construct proper time.Time values (with Location set).
//   - It detects all-day events by inspecting the DTSTART value format.
//   - It records RRULE/EXDATE/RECURRENCE-ID but does not expand recurrences;
//     expansion is done in internal/ics/expand.go.
func ParseICS(src Source, body []byte) ([]ParsedEvent, error) {
	if len(body) == 0 {
		return nil, errors.New("empty ICS body")
	}

	cal, err := ical.ParseCalendar(bytes.NewReader(body))
	if err != nil {
		appLog.Error("ics parse failed", err, "id", src.ID, "url", redactURL(src.URL))
		return nil, err
	}

	events := make([]ParsedEvent, 0)

	for _, comp := range cal.Events() {
		ev, perr := parseVEvent(src, comp)
		if perr != nil {
			// Log and skip this event, but keep parsing others.
			appLog.Error("ics vevent parse failed", perr, "id", src.ID, "url", redactURL(src.URL))
			continue
		}
		events = append(events, ev)
	}

	appLog.Info("ics parse completed", "id", src.ID, "url", redactURL(src.URL), "event_count", len(events))
	return events, nil
}

func parseVEvent(src Source, ve *ical.VEvent) (ParsedEvent, error) {
	var out ParsedEvent
	out.Source = src

	// UID
	uidProp := ve.GetProperty(ical.ComponentPropertyUniqueId)
	if uidProp == nil || uidProp.Value == "" {
		return out, errors.New("missing UID")
	}
	out.UID = uidProp.Value

	// SEQUENCE (optional, used for overrides/versioning)
	if seqProp := ve.GetProperty(ical.ComponentPropertySequence); seqProp != nil {
		if n, err := strconv.Atoi(strings.TrimSpace(seqProp.Value)); err == nil {
			out.Seq = n
		}
	}

	// Summary / Description / Location
	if p := ve.GetProperty(ical.ComponentPropertySummary); p != nil {
		out.Summary = p.Value
	}
	if p := ve.GetProperty(ical.ComponentPropertyDescription); p != nil {
		out.Description = p.Value
	}
	if p := ve.GetProperty(ical.ComponentPropertyLocation); p != nil {
		out.Location = p.Value
	}

	// DTSTART / DTEND. We use the library's helpers for timezone logic.
	// NOTE: GetStartAt/GetEndAt in arran4/golang-ical 리턴 타입이 time.Time 하나이므로
	// 단일 변수 할당만 사용해야 한다.
	start, _ := ve.GetStartAt()
	end, _ := ve.GetEndAt()

	out.Start = start
	out.End = end

	// Detect all-day: if DTSTART has VALUE=DATE or is in YYYYMMDD form
	allDay := false
	if dtStartProp := ve.GetProperty(ical.ComponentPropertyDtStart); dtStartProp != nil {
		val := dtStartProp.Value
		// VALUE=DATE or no 'T' in the value -> all-day
		if params := dtStartProp.ICalParameters; params != nil {
			if vs, ok := params["VALUE"]; ok && len(vs) > 0 && strings.EqualFold(vs[0], "DATE") {
				allDay = true
			}
		}
		if !strings.Contains(val, "T") {
			allDay = true
		}
		// Capture TZID if present.
		if params := dtStartProp.ICalParameters; params != nil {
			if tzs, ok := params["TZID"]; ok && len(tzs) > 0 {
				out.StartTZ = tzs[0]
			}
		}
	}

	if dtEndProp := ve.GetProperty(ical.ComponentPropertyDtEnd); dtEndProp != nil {
		if params := dtEndProp.ICalParameters; params != nil {
			if tzs, ok := params["TZID"]; ok && len(tzs) > 0 {
				out.EndTZ = tzs[0]
			}
		}
	}

	out.AllDay = allDay

	// RRULE (we only keep raw string here; expansion will be in expand.go).
	if rruleProp := ve.GetProperty(ical.ComponentPropertyRrule); rruleProp != nil {
		out.RawRRule = rruleProp.Value
	}

	// EXDATE (can appear multiple times)
	// Use string property name to avoid dependency on constant variants.
	exProps := ve.GetProperties(ical.ComponentPropertyExdate)
	for _, p := range exProps {
		val := p.Value
		if val == "" {
			continue
		}
		parts := strings.Split(val, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			// This parse is relatively naive; for better correctness we
			// would inspect VALUE/TZID like above. For now, rely on a
			// helper that handles basic DATE/DATE-TIME/UTC forms.
			if t, err := parseICSTime(part); err == nil {
				out.ExDates = append(out.ExDates, t)
			}
		}
	}

	// RECURRENCE-ID (overridden instance)
	// Use raw property name to avoid constant mismatch.
	if ridProp := ve.GetProperty("RECURRENCE-ID"); ridProp != nil {
		if t, err := parseICSTime(ridProp.Value); err == nil {
			out.Recurrence = &t
			out.IsOverride = true
		}
	}

	return out, nil
}

// parseICSTime parses a basic ICS date/date-time string into time.Time.
// NOTE: This is a simplified helper for EXDATE/RECURRENCE-ID where we do
// not yet have full parameter context. Expansion logic will handle tz
// normalization later.
func parseICSTime(v string) (time.Time, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}, errors.New("empty time value")
	}

	// UTC form, e.g., 20250101T090000Z
	if strings.HasSuffix(v, "Z") {
		const layout = "20060102T150405Z"
		return time.Parse(layout, v)
	}

	// Local date-time, e.g., 20250101T090000
	if strings.Contains(v, "T") {
		const layout = "20060102T150405"
		return time.ParseInLocation(layout, v, time.Local)
	}

	// Date-only (all-day), e.g., 20250101
	const layoutDate = "20060102"
	return time.ParseInLocation(layoutDate, v, time.Local)
}
