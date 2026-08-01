package outreach

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/google/uuid"
)

func path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".nexus", "outreach.json"), nil
}

func Load() ([]Item, error) {
	p, err := path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var doc StoreFile
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	sort.SliceStable(doc.Items, func(i, j int) bool {
		return doc.Items[i].UpdatedAt.After(doc.Items[j].UpdatedAt)
	})
	return doc.Items, nil
}

func saveAll(items []Item) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, ".nexus")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	p, err := path()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(StoreFile{Items: items}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0600)
}

// SaveAll persists the full outreach item set (exported for the API layer,
// e.g. the A/B variant tag endpoint).
func SaveAll(items []Item) error { return saveAll(items) }

func Upsert(item Item) error {
	items, err := Load()
	if err != nil {
		return err
	}
	now := time.Now()
	if item.ID == "" {
		item.ID = uuid.NewString()
		item.CreatedAt = now
	}
	item.UpdatedAt = now
	found := false
	for i := range items {
		if items[i].ID == item.ID {
			if item.CreatedAt.IsZero() {
				item.CreatedAt = items[i].CreatedAt
			}
			items[i] = item
			found = true
			break
		}
	}
	if !found {
		items = append(items, item)
	}
	return saveAll(items)
}

func Delete(id string) error {
	items, err := Load()
	if err != nil {
		return err
	}
	out := items[:0]
	for _, it := range items {
		if it.ID != id {
			out = append(out, it)
		}
	}
	if len(out) == len(items) {
		return fmt.Errorf("outreach item not found")
	}
	return saveAll(out)
}

func CountSentToday(ch Channel) (int, error) {
	items, err := Load()
	if err != nil {
		return 0, err
	}
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	n := 0
	for _, it := range items {
		if it.Channel != ch || !countedAsSent(it.Status) {
			continue
		}
		if !it.SentAt.IsZero() && !it.SentAt.Before(start) {
			n++
		}
	}
	return n, nil
}

// countedAsSent reports whether the status implies a real send happened.
// Follow-up states count too — an item that sent follow-up #1 today sits in
// followup_due, and it must still count toward the daily cap.
func countedAsSent(s Status) bool {
	switch s {
	case StatusSent, StatusFollowUpDue, StatusSequenceDone, StatusReplied, StatusOpened:
		return true
	default:
		return false
	}
}
