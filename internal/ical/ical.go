// Package ical writes a minimal iCalendar (RFC 5545) feed.
//
// Hand-written rather than pulled in as a dependency: the feed has six fields
// and the format's only real subtleties are line folding and text escaping,
// both handled here. Getting either wrong is invisible in a text editor and
// fatal in Google Calendar, so they are the parts worth reading carefully.
package ical

import (
	"fmt"
	"strings"
	"time"
)

// Calendar is a feed under construction.
type Calendar struct {
	buf  strings.Builder
	name string
}

// New starts a calendar with the given display name.
func New(name string) *Calendar {
	c := &Calendar{name: name}
	c.line("BEGIN", "VCALENDAR")
	c.line("VERSION", "2.0")
	c.line("PRODID", "-//Itemize NTNU//itemize.no//NO")
	c.line("CALSCALE", "GREGORIAN")
	c.line("METHOD", "PUBLISH")
	// X-WR-CALNAME is not in the specification, but it is what Google and
	// Apple read to label a subscribed calendar.
	c.line("X-WR-CALNAME", name)
	c.line("X-WR-TIMEZONE", "Europe/Oslo")
	return c
}

// Event is one entry in the feed.
type Event struct {
	UID         string
	Start       time.Time
	End         time.Time
	Stamp       time.Time
	Summary     string
	Description string
	Location    string
	URL         string
}

// Add appends an event.
func (c *Calendar) Add(e Event) {
	stamp := e.Stamp
	if stamp.IsZero() {
		stamp = time.Now()
	}

	c.line("BEGIN", "VEVENT")
	c.line("UID", e.UID)
	c.line("DTSTAMP", utc(stamp))
	c.line("DTSTART", utc(e.Start))
	if !e.End.IsZero() {
		c.line("DTEND", utc(e.End))
	}
	c.line("SUMMARY", escape(e.Summary))
	if e.Description != "" {
		c.line("DESCRIPTION", escape(e.Description))
	}
	if e.Location != "" {
		c.line("LOCATION", escape(e.Location))
	}
	if e.URL != "" {
		// URLs are not TEXT and must not be escaped — a comma or semicolon in
		// a query string is significant.
		c.line("URL", e.URL)
	}
	c.line("END", "VEVENT")
}

// String closes the calendar and returns the feed.
func (c *Calendar) String() string {
	out := c.buf.String()
	return out + fold("END:VCALENDAR")
}

// ContentType is what to serve the feed as.
const ContentType = "text/calendar; charset=utf-8"

func (c *Calendar) line(name, value string) {
	c.buf.WriteString(fold(name + ":" + value))
}

// utc renders a timestamp in the format RFC 5545 calls a UTC date-time.
// Everything is emitted in UTC so no VTIMEZONE component is needed.
func utc(t time.Time) string {
	return t.UTC().Format("20060102T150405Z")
}

// escape quotes the characters that are structural in a TEXT value.
//
// Order matters: the backslash has to be doubled first, or the escapes added
// afterwards would be escaped in turn.
func escape(s string) string {
	r := strings.NewReplacer(
		`\`, `\\`,
		"\r\n", `\n`,
		"\n", `\n`,
		"\r", `\n`,
		`;`, `\;`,
		`,`, `\,`,
	)
	return r.Replace(s)
}

// fold breaks a content line so no line exceeds 75 octets, per RFC 5545
// section 3.1, and terminates it with CRLF.
//
// The limit counts octets, not characters, so the split has to respect UTF-8
// boundaries — cutting a multi-byte character in half produces a feed that
// some clients reject outright and others render as mojibake. Norwegian event
// titles make that a live concern rather than a theoretical one.
//
// A continuation line begins with a single space, which the parser strips.
func fold(line string) string {
	const limit = 75

	var out strings.Builder
	remaining := line
	first := true

	for {
		budget := limit
		if !first {
			budget-- // the leading space counts toward the octet limit
		}
		if len(remaining) <= budget {
			if !first {
				out.WriteByte(' ')
			}
			out.WriteString(remaining)
			out.WriteString("\r\n")
			return out.String()
		}

		cut := budget
		// Back up to a rune boundary. Continuation bytes are 10xxxxxx.
		for cut > 0 && remaining[cut]&0xC0 == 0x80 {
			cut--
		}
		if cut == 0 {
			// A single rune wider than the budget; emit it whole rather than
			// splitting it, and accept the over-long line.
			cut = budget
		}

		if !first {
			out.WriteByte(' ')
		}
		out.WriteString(remaining[:cut])
		out.WriteString("\r\n")
		remaining = remaining[cut:]
		first = false
	}
}

// UID builds a globally unique identifier for an event.
func UID(id string) string { return fmt.Sprintf("%s@itemize.no", id) }
