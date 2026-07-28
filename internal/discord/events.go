package discord

import (
	"context"
	"strings"
	"time"
)

// Discord's own limits on a scheduled event.
const (
	// eventNameMax is 100. Our own name limit is 50, so this never binds —
	// but it is stated rather than assumed.
	eventNameMax = 100
	// eventDescriptionMax is 1000, while an event's info field allows 2000 and
	// the venue and CTF lines are prepended on top of it. The previous version
	// sent the lot and Discord rejected the request, which surfaced to the
	// board as "Error upserting discord event" with no explanation.
	eventDescriptionMax = 1000
)

// Entity types and privacy levels, from the Discord API documentation.
const (
	entityTypeExternal = 3
	privacyGuildOnly   = 2
)

// ScheduledEvent is the subset of an event Discord needs.
type ScheduledEvent struct {
	Name        string
	Description string
	Location    string
	Start       time.Time
	End         time.Time
}

type scheduledEventPayload struct {
	Name           string         `json:"name"`
	Description    string         `json:"description"`
	EntityMetadata entityMetadata `json:"entity_metadata"`
	StartTime      string         `json:"scheduled_start_time"`
	EndTime        string         `json:"scheduled_end_time"`
	EntityType     int            `json:"entity_type"`
	PrivacyLevel   int            `json:"privacy_level"`
}

type entityMetadata struct {
	Location string `json:"location"`
}

type scheduledEventResponse struct {
	ID string `json:"id"`
}

// UpsertScheduledEvent creates or updates the guild event and returns its id.
//
// An existing id means update; an empty one means create. The caller is
// responsible for persisting the returned id — losing it orphans the event,
// since there is then no handle to update or delete it by.
func (c *Client) UpsertScheduledEvent(ctx context.Context, existingID string, e ScheduledEvent) (string, error) {
	payload := scheduledEventPayload{
		Name:           truncate(e.Name, eventNameMax),
		Description:    truncate(e.Description, eventDescriptionMax),
		EntityMetadata: entityMetadata{Location: e.Location},
		StartTime:      e.Start.Format(time.RFC3339),
		EndTime:        e.End.Format(time.RFC3339),
		EntityType:     entityTypeExternal,
		PrivacyLevel:   privacyGuildOnly,
	}

	path := "/guilds/" + c.cfg.GuildID + "/scheduled-events"
	method := "POST"
	if existingID != "" {
		path += "/" + existingID
		method = "PATCH"
	}

	var out scheduledEventResponse
	if err := c.do(ctx, method, path, payload, &out); err != nil {
		return "", err
	}
	return out.ID, nil
}

// DeleteScheduledEvent removes a guild event. An event that is already gone is
// not an error — the desired state has been reached either way.
func (c *Client) DeleteScheduledEvent(ctx context.Context, id string) error {
	if id == "" {
		return nil
	}
	err := c.do(ctx, "DELETE", "/guilds/"+c.cfg.GuildID+"/scheduled-events/"+id, nil, nil)
	if apiErr, ok := err.(*APIError); ok && apiErr.Status == 404 {
		return nil
	}
	return err
}

// BuildDescription assembles the event description Discord shows, putting the
// practical details above the prose.
func BuildDescription(info, locationURL, registerURL, ctfName, ctfURL string) string {
	var lines []string
	if registerURL != "" {
		// The previous version used the venue URL here rather than the
		// registration one, so anyone following "Registrering" landed on a map.
		lines = append(lines, "Registrering: "+registerURL)
	}
	if locationURL != "" {
		lines = append(lines, "Hvor: "+locationURL)
	}
	if ctfName != "" {
		if ctfURL != "" {
			lines = append(lines, "CTF: "+ctfName+" ("+ctfURL+")")
		} else {
			lines = append(lines, "CTF: "+ctfName)
		}
	}
	if len(lines) == 0 {
		return info
	}
	lines = append(lines, strings.Repeat("-", 50), "", info)
	return strings.Join(lines, "\n")
}

// truncate shortens text to at most max runes, marking that it was cut.
func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-1]) + "…"
}
