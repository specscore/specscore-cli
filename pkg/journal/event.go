package journal

import (
	"fmt"
	"time"
)

// Event is one journal record. The flat fields are ingitdb `--where`-friendly;
// Data carries the type-specific payload verbatim.
type Event struct {
	Type         string
	Timestamp    time.Time
	MachineID    string
	SourceRepo   string
	SourceOrigin string // omitted from the record when empty
	Stream       string
	Data         map[string]any // omitted from the record when nil
}

// keyTimeLayout is a filesystem-safe (colon-free) RFC3339-style timestamp used
// in the record key / filename. Date sharding lives in the leading path segments.
const keyTimeLayout = "2006-01-02T15-04-05.000000000Z"

// eventKey returns the date-sharded record key
// `YYYY/MM/DD/<timestamp>-<machine>-<uuid>`. inGitDB maps slashes in the key to
// directories, so this yields events/$records/YYYY/MM/DD/<file>.json on disk.
func eventKey(t time.Time, machineID, shortUUID string) string {
	u := t.UTC()
	return fmt.Sprintf("%04d/%02d/%02d/%s-%s-%s",
		u.Year(), int(u.Month()), u.Day(), u.Format(keyTimeLayout), machineID, shortUUID)
}

// toMap renders the event into the persisted record shape. source_origin and
// data are omitted when empty/nil.
func (e Event) toMap() map[string]any {
	m := map[string]any{
		"type":        e.Type,
		"timestamp":   e.Timestamp.UTC().Format(time.RFC3339Nano),
		"machine_id":  e.MachineID,
		"source_repo": e.SourceRepo,
		"stream":      e.Stream,
	}
	if e.SourceOrigin != "" {
		m["source_origin"] = e.SourceOrigin
	}
	if e.Data != nil {
		m["data"] = e.Data
	}
	return m
}
