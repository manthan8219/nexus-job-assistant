package inbox

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/manthan8219/nexus-job-assistant/internal/nexusdir"
)

// StoreFile is the on-disk shape of the highlights store.
type StoreFile struct {
	Highlights []Highlight `json:"highlights"`
}

// HighlightsPath returns the JSON store path for the highlights.
func HighlightsPath() (string, error) {
	return filepath.Join(nexusdir.Home(), "highlights.json"), nil
}

// LoadAll reads the highlights store. A missing file is an empty store.
func LoadAll(path string) ([]Highlight, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var doc StoreFile
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("inbox: decode highlights: %w", err)
	}
	return doc.Highlights, nil
}

// SaveAll writes the highlights store.
func SaveAll(path string, highlights []Highlight) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(StoreFile{Highlights: highlights}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// Upsert adds or replaces a highlight, deduplicated by Message-ID (falling
// back to from + subject + date-minute).
func Upsert(path string, h Highlight) error {
	hs, err := LoadAll(path)
	if err != nil {
		return err
	}
	key := dedupKey(h)
	for i := range hs {
		if dedupKey(hs[i]) == key {
			hs[i] = h
			return SaveAll(path, hs)
		}
	}
	hs = append(hs, h)
	return SaveAll(path, hs)
}

func dedupKey(h Highlight) string {
	if h.MessageID != "" {
		return "id:" + h.MessageID
	}
	return "k:" + strings.ToLower(h.From) + "|" + strings.ToLower(h.Subject) + "|" + h.Date.Format("2006-01-02T15:04")
}
