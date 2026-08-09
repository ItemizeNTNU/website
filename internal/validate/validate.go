// Package validate provides the small set of checks the forms need.
//
// Hand-written rather than a validation library, for three reasons drawn from
// what is actually being validated here:
//
//   - The registration form's fields are conditional on membership type, and
//     the inactive ones must be *dropped* rather than merely ignored — they go
//     straight into FusionAuth, and an alumnus must not arrive carrying an
//     empty study year. Tag-based validators express "required when" but have
//     no notion of stripping, so that logic would be hand-written anyway.
//   - The messages are user-visible Norwegian prose. Getting one custom
//     sentence out of a tag-driven library means a translator registry.
//   - The whole surface is about fifteen fields. This file plus its callers is
//     under two hundred lines, has no dependencies, and is exhaustively
//     table-testable — which suits a codebase meant to be picked up by a new
//     board every year better than a reflection-driven DSL would.
package validate

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// Errors collects field problems. The zero value is ready to use.
type Errors map[string]string

// Add records a problem, keeping the first message for a field — later checks
// on an already-failed field are usually noise caused by the first failure.
func (e *Errors) Add(field, msg string) {
	if *e == nil {
		*e = Errors{}
	}
	if _, exists := (*e)[field]; !exists {
		(*e)[field] = msg
	}
}

// Any reports whether anything failed.
func (e Errors) Any() bool { return len(e) > 0 }

// First returns one message, chosen deterministically.
//
// The previous API validated with abortEarly and returned a single message, so
// the JSON endpoints still do. Sorting matters: Go map iteration is random, and
// an error message that changes between identical requests is untestable and
// bewildering to debug.
func (e Errors) First() string {
	if len(e) == 0 {
		return ""
	}
	fields := make([]string, 0, len(e))
	for field := range e {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	return e[fields[0]]
}

// Error implements error so a validation failure can be returned as one.
func (e Errors) Error() string { return e.First() }

// Text trims and length-checks a required field.
func (e *Errors) Text(field, label, v string, min, max int) string {
	v = strings.TrimSpace(v)
	switch {
	case v == "":
		e.Add(field, label+" må fylles ut.")
	case len([]rune(v)) < min:
		e.Add(field, fmt.Sprintf("%s må være minst %d tegn.", label, min))
	case len([]rune(v)) > max:
		e.Add(field, fmt.Sprintf("%s kan ikke være lengre enn %d tegn.", label, max))
	}
	return v
}

// Optional trims and length-checks a field that may be empty.
func (e *Errors) Optional(field, label, v string, max int) string {
	v = strings.TrimSpace(v)
	if len([]rune(v)) > max {
		e.Add(field, fmt.Sprintf("%s kan ikke være lengre enn %d tegn.", label, max))
	}
	return v
}

// HTTPSURL checks a URL that may be empty.
//
// The two messages are reproduced verbatim from the previous server, in
// English, because they are surfaced directly in the event form and changing
// them for no reason is churn.
func (e *Errors) HTTPSURL(field, v string) string {
	// Trimmed first, matching the old schema's ordering: a leading space is
	// stripped rather than rejected as whitespace.
	v = strings.TrimSpace(v)
	if v == "" {
		return v
	}
	if !strings.HasPrefix(v, "https://") {
		e.Add(field, "the URL needs to start with https://")
		return v
	}
	// JavaScript's \s covers more than a plain space — tabs, newlines,
	// non-breaking spaces, every Unicode separator. unicode.IsSpace is the
	// near-equivalent, and a pasted URL really can carry a non-breaking space.
	if strings.IndexFunc(v, unicode.IsSpace) >= 0 {
		e.Add(field, "the URL can not include whitespace")
	}
	return v
}

// Number parses a non-negative decimal.
func (e *Errors) Number(field, label, v string, min, max float64) float64 {
	v = strings.TrimSpace(v)
	if v == "" {
		e.Add(field, label+" må fylles ut.")
		return 0
	}
	// Accept the comma decimal separator; a Norwegian keyboard produces it and
	// rejecting it is a needless papercut.
	n, err := strconv.ParseFloat(strings.Replace(v, ",", ".", 1), 64)
	// "NaN", "Inf" and "-Inf" all parse, and the range check below cannot hold
	// them: NaN compares false against both bounds, so it passes through
	// untouched however narrow the range is. The only caller is the event
	// duration, where NaN becomes an end time in the eighteenth century — the
	// event reads as already finished and vanishes from the listing the moment
	// it is saved. They are not numbers a form can mean, so they are refused
	// with the same message as any other unparseable value.
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		e.Add(field, label+" må være et tall.")
		return 0
	}
	switch {
	case n < min:
		e.Add(field, fmt.Sprintf("%s kan ikke være mindre enn %g.", label, min))
	case n > max:
		e.Add(field, fmt.Sprintf("%s kan ikke være større enn %g.", label, max))
	}
	return n
}

// Int parses a whole number within a range.
func (e *Errors) Int(field, label, v string, min, max int) int {
	v = strings.TrimSpace(v)
	if v == "" {
		e.Add(field, label+" må fylles ut.")
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		e.Add(field, label+" må være et heltall.")
		return 0
	}
	switch {
	case n < min:
		e.Add(field, fmt.Sprintf("%s kan ikke være mindre enn %d.", label, min))
	case n > max:
		e.Add(field, fmt.Sprintf("%s kan ikke være større enn %d.", label, max))
	}
	return n
}

// OneOf checks membership in a fixed set.
func (e *Errors) OneOf(field, label, v string, allowed ...string) string {
	v = strings.TrimSpace(v)
	for _, a := range allowed {
		if v == a {
			return v
		}
	}
	e.Add(field, label+" er ikke et gyldig valg.")
	return ""
}
