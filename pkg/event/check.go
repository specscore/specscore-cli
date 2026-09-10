package event

import "sort"

// LedgerUUIDs parses JSONL event-ledger bytes using the same parsing and
// validation as MergeLedgers/MergeLedgersWithOptions (parseLedger) and
// returns the set of event UUIDs present. `data` may be nil or empty — an
// absent or empty ledger yields an empty set, not an error, matching the
// "no ledger there yet" cases callers such as `specscore event check` must
// tolerate. `path` is used only to make a parse-error message point at a
// meaningful location: pass a filesystem path for a working-tree read, or a
// synthetic `<ref>:<repo-relative-path>` label for a historical read.
func LedgerUUIDs(data []byte, path string) (map[string]struct{}, error) {
	records, err := parseLedger(data, path)
	if err != nil {
		return nil, err
	}
	uuids := make(map[string]struct{}, len(records))
	for _, record := range records {
		uuids[record.event.UUID] = struct{}{}
	}
	return uuids, nil
}

// MissingUUIDs returns, in ascending sorted order, every UUID present in
// base that is absent from working. It is the core check behind `specscore
// event check`: an append-only ledger must never lose an event a prior
// (base) revision already had, so a non-empty result here means the
// working-tree ledger went backwards relative to base.
func MissingUUIDs(base, working map[string]struct{}) []string {
	missing := make([]string, 0, len(base))
	for id := range base {
		if _, ok := working[id]; !ok {
			missing = append(missing, id)
		}
	}
	sort.Strings(missing)
	return missing
}
