package workcontext

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/google/uuid"
)

func dir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".nexus"), nil
}

func path() (string, error) {
	d, err := dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "work_context.json"), nil
}

// Load all projects (empty slice if none yet).
func Load() ([]Project, error) {
	p, err := path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return []Project{}, nil
		}
		return nil, err
	}
	var doc StoreFile
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	sort.SliceStable(doc.Projects, func(i, j int) bool {
		return doc.Projects[i].UpdatedAt.After(doc.Projects[j].UpdatedAt)
	})
	return doc.Projects, nil
}

func saveAll(projects []Project) error {
	d, err := dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(d, 0700); err != nil {
		return err
	}
	p, err := path()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(StoreFile{Projects: projects}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0600)
}

// Upsert inserts or replaces a project by ID.
func Upsert(proj Project) error {
	projects, err := Load()
	if err != nil {
		return err
	}
	now := time.Now()
	if proj.ID == "" {
		proj.ID = uuid.NewString()
		proj.CreatedAt = now
	}
	proj.UpdatedAt = now
	if proj.Source == "" {
		proj.Source = "manual"
	}
	if len(proj.Bullets) == 0 {
		proj.Bullets = ExtractBullets(proj.Summary)
	}

	found := false
	for i := range projects {
		if projects[i].ID == proj.ID {
			if proj.CreatedAt.IsZero() {
				proj.CreatedAt = projects[i].CreatedAt
			}
			projects[i] = proj
			found = true
			break
		}
	}
	if !found {
		if proj.CreatedAt.IsZero() {
			proj.CreatedAt = now
		}
		projects = append(projects, proj)
	}
	return saveAll(projects)
}

// Delete removes a project by ID.
func Delete(id string) error {
	projects, err := Load()
	if err != nil {
		return err
	}
	out := projects[:0]
	for _, p := range projects {
		if p.ID != id {
			out = append(out, p)
		}
	}
	if len(out) == len(projects) {
		return fmt.Errorf("project not found")
	}
	return saveAll(out)
}

// Get returns one project.
func Get(id string) (Project, bool, error) {
	projects, err := Load()
	if err != nil {
		return Project{}, false, err
	}
	for _, p := range projects {
		if p.ID == id {
			return p, true, nil
		}
	}
	return Project{}, false, nil
}
