package companies

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/nexusdir"
	_ "modernc.org/sqlite"
)

// DB is the local company footprint database (~/.nexus/companies.db by default).
type DB struct {
	db *sql.DB
}

// defaultDBPath returns ~/.nexus/companies.db, creating the directory if needed.
func defaultDBPath() (string, error) {
	dir := nexusdir.Home()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	return filepath.Join(dir, "companies.db"), nil
}

// OpenDefault opens ~/.nexus/companies.db (creates dir + schema), seeding the
// embedded catalogs and kicking off the background network seed.
func OpenDefault() (*DB, error) {
	path, err := defaultDBPath()
	if err != nil {
		return nil, err
	}
	db, err := Open(path)
	if err != nil {
		return nil, err
	}
	db.ensureSeeded(path)
	return db, nil
}

// OpenDefaultEmbedded opens ~/.nexus/companies.db and seeds only the embedded
// catalogs (boards + India employers) — no network fetch. Used by the API
// server so startup is deterministic and offline-friendly.
func OpenDefaultEmbedded() (*DB, error) {
	path, err := defaultDBPath()
	if err != nil {
		return nil, err
	}
	return openEmbedded(path)
}

// OpenEmbeddedAt opens (or creates) a company DB at path, seeded from the
// embedded catalogs only (no network fetch). Per-user islands use it so each
// user's footprint database is offline-seeded.
func OpenEmbeddedAt(path string) (*DB, error) {
	return openEmbedded(path)
}

// openEmbedded opens a company DB at path and seeds only the embedded
// catalogs — no network fetch.
func openEmbedded(path string) (*DB, error) {
	db, err := Open(path)
	if err != nil {
		return nil, err
	}
	db.seedEmbeddedOnly()
	return db, nil
}

// CountBySource returns how many companies came from a given source tag
// (e.g. "ycombinator", "openjobs").
func (s *DB) CountBySource(source string) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(1) FROM companies WHERE source = ?`, source).Scan(&n)
	return n, err
}

// Open opens (or creates) a SQLite company DB at path.
func Open(path string) (*DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		db.Close()
		return nil, err
	}
	// Wait (up to 5s) for other writers (e.g. the background network seed)
	// instead of failing immediately with SQLITE_BUSY.
	if _, err := db.Exec(`PRAGMA busy_timeout = 5000`); err != nil {
		db.Close()
		return nil, err
	}
	s := &DB{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *DB) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *DB) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS companies (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  website TEXT NOT NULL DEFAULT '',
  ats TEXT NOT NULL DEFAULT '',
  board TEXT NOT NULL DEFAULT '',
  board_url TEXT NOT NULL DEFAULT '',
  hire_countries TEXT NOT NULL DEFAULT '[]',
  hire_country_codes TEXT NOT NULL DEFAULT '[]',
  hq_country TEXT NOT NULL DEFAULT '',
  hq_country_code TEXT NOT NULL DEFAULT '',
  kind TEXT NOT NULL DEFAULT '',
  industry TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL,
  UNIQUE(name, board_url)
);
CREATE INDEX IF NOT EXISTS idx_companies_name ON companies(name);
CREATE TABLE IF NOT EXISTS company_countries (
  company_id INTEGER NOT NULL,
  country_key TEXT NOT NULL,
  PRIMARY KEY (company_id, country_key),
  FOREIGN KEY (company_id) REFERENCES companies(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_company_countries_key ON company_countries(country_key);
`)
	return err
}

// Count returns total companies.
func (s *DB) Count() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(1) FROM companies`).Scan(&n)
	return n, err
}

// CountByCountry returns how many companies match a country name or code.
func (s *DB) CountByCountry(country string) (int, error) {
	key := CountryKey(country)
	if key == "" {
		return 0, fmt.Errorf("empty country")
	}
	var n int
	err := s.db.QueryRow(`SELECT COUNT(DISTINCT company_id) FROM company_countries WHERE country_key = ?`, key).Scan(&n)
	return n, err
}

// FindByCountry returns companies that hire in / are associated with the given country.
// Accepts "IN", "India", "india", etc. Results are ordered by name.
func (s *DB) FindByCountry(country string) ([]Company, error) {
	return s.FindByCountryLimit(country, 0)
}

// FindByCountryLimit is FindByCountry with an optional max rows (0 = no limit).
func (s *DB) FindByCountryLimit(country string, limit int) ([]Company, error) {
	key := CountryKey(country)
	if key == "" {
		return nil, fmt.Errorf("empty country")
	}
	q := `
SELECT c.id, c.name, c.website, c.ats, c.board, c.board_url,
       c.hire_countries, c.hire_country_codes, c.hq_country, c.hq_country_code,
       c.kind, c.industry, c.source, c.updated_at
FROM companies c
JOIN company_countries cc ON cc.company_id = c.id
WHERE cc.country_key = ?
ORDER BY c.name COLLATE NOCASE`
	args := []any{key}
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list, err := scanCompanies(rows)
	if err != nil {
		return nil, err
	}
	return dedupeCompaniesByName(list), nil
}

// Upsert inserts or updates a company and rebuilds its country index rows.
func (s *DB) Upsert(c Company) error {
	if strings.TrimSpace(c.Name) == "" {
		return fmt.Errorf("company name required")
	}
	if c.UpdatedAt.IsZero() {
		c.UpdatedAt = time.Now().UTC()
	}
	codes := c.HireCountryCodes
	names := c.HireCountries
	if len(codes) == 0 && len(names) > 0 {
		for _, n := range names {
			_, iso, ok := NormalizeCountry(n)
			if ok && iso != "" {
				codes = append(codes, iso)
			}
		}
		c.HireCountryCodes = uniqueFold(codes)
	}
	hcJSON, _ := json.Marshal(c.HireCountries)
	ccJSON, _ := json.Marshal(c.HireCountryCodes)

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	res, err := tx.Exec(`
INSERT INTO companies (
  name, website, ats, board, board_url, hire_countries, hire_country_codes,
  hq_country, hq_country_code, kind, industry, source, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(name, board_url) DO UPDATE SET
  website=CASE WHEN excluded.website != '' THEN excluded.website ELSE companies.website END,
  ats=CASE WHEN excluded.ats != '' THEN excluded.ats ELSE companies.ats END,
  board=CASE WHEN excluded.board != '' THEN excluded.board ELSE companies.board END,
  hire_countries=companies.hire_countries,
  hire_country_codes=companies.hire_country_codes,
  hq_country=CASE WHEN excluded.hq_country != '' THEN excluded.hq_country ELSE companies.hq_country END,
  hq_country_code=CASE WHEN excluded.hq_country_code != '' THEN excluded.hq_country_code ELSE companies.hq_country_code END,
  kind=CASE WHEN excluded.kind != '' THEN excluded.kind ELSE companies.kind END,
  industry=CASE WHEN excluded.industry != '' THEN excluded.industry ELSE companies.industry END,
  source=CASE WHEN excluded.source != '' THEN excluded.source ELSE companies.source END,
  updated_at=excluded.updated_at
`, c.Name, c.Website, c.ATS, c.Board, c.BoardURL, string(hcJSON), string(ccJSON),
		c.HQCountry, c.HQCountryCode, c.Kind, c.Industry, c.Source, c.UpdatedAt.UTC().Format(time.RFC3339))
	if err != nil {
		return err
	}

	var id int64
	err = tx.QueryRow(`SELECT id FROM companies WHERE name = ? AND board_url = ?`, c.Name, c.BoardURL).Scan(&id)
	if err != nil {
		// fallback if board_url empty conflict path
		_ = res
		err = tx.QueryRow(`SELECT id FROM companies WHERE name = ? ORDER BY id DESC LIMIT 1`, c.Name).Scan(&id)
		if err != nil {
			return err
		}
	}

	// Re-read stored country fields, then UNION with incoming so India-priority
	// (and manual adds) extend OpenJobs tags instead of replacing them.
	var hcRaw, ccRaw, hqC, hqCode string
	if err := tx.QueryRow(`SELECT hire_countries, hire_country_codes, hq_country, hq_country_code FROM companies WHERE id = ?`, id).Scan(&hcRaw, &ccRaw, &hqC, &hqCode); err != nil {
		return err
	}
	var hireNames, hireCodes []string
	_ = json.Unmarshal([]byte(hcRaw), &hireNames)
	_ = json.Unmarshal([]byte(ccRaw), &hireCodes)
	hireNames = uniqueFold(append(hireNames, names...))
	hireCodes = uniqueFold(append(hireCodes, codes...))
	hcJSON2, _ := json.Marshal(hireNames)
	ccJSON2, _ := json.Marshal(hireCodes)
	if _, err := tx.Exec(`UPDATE companies SET hire_countries = ?, hire_country_codes = ? WHERE id = ?`, string(hcJSON2), string(ccJSON2), id); err != nil {
		return err
	}

	if _, err := tx.Exec(`DELETE FROM company_countries WHERE company_id = ?`, id); err != nil {
		return err
	}
	keys := map[string]struct{}{}
	for _, code := range hireCodes {
		keys[CountryKey(code)] = struct{}{}
	}
	for _, name := range hireNames {
		keys[CountryKey(name)] = struct{}{}
	}
	if hqCode != "" {
		keys[CountryKey(hqCode)] = struct{}{}
	}
	if hqC != "" {
		keys[CountryKey(hqC)] = struct{}{}
	}
	for k := range keys {
		if k == "" {
			continue
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO company_countries(company_id, country_key) VALUES (?, ?)`, id, k); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func scanCompanies(rows *sql.Rows) ([]Company, error) {
	var out []Company
	for rows.Next() {
		var c Company
		var hc, cc, updated string
		if err := rows.Scan(
			&c.ID, &c.Name, &c.Website, &c.ATS, &c.Board, &c.BoardURL,
			&hc, &cc, &c.HQCountry, &c.HQCountryCode, &c.Kind, &c.Industry, &c.Source, &updated,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(hc), &c.HireCountries)
		_ = json.Unmarshal([]byte(cc), &c.HireCountryCodes)
		if t, err := time.Parse(time.RFC3339, updated); err == nil {
			c.UpdatedAt = t
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func uniqueFold(in []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		k := strings.ToUpper(s)
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, s)
	}
	return out
}

// Search finds companies by name/website/board/ats substring.
// If country is non-empty, results are restricted to that country footprint.
func (s *DB) Search(query, country string, limit int) ([]Company, error) {
	if limit <= 0 {
		limit = 2000
	}
	q := strings.ToLower(strings.TrimSpace(query))
	like := "%" + q + "%"

	var (
		rows *sql.Rows
		err  error
	)
	if ctry := strings.TrimSpace(country); ctry != "" {
		key := CountryKey(ctry)
		if q == "" {
			rows, err = s.db.Query(`
SELECT c.id, c.name, c.website, c.ats, c.board, c.board_url,
       c.hire_countries, c.hire_country_codes, c.hq_country, c.hq_country_code,
       c.kind, c.industry, c.source, c.updated_at
FROM companies c
JOIN company_countries cc ON cc.company_id = c.id
WHERE cc.country_key = ?
ORDER BY c.name COLLATE NOCASE
LIMIT ?`, key, limit)
		} else {
			rows, err = s.db.Query(`
SELECT c.id, c.name, c.website, c.ats, c.board, c.board_url,
       c.hire_countries, c.hire_country_codes, c.hq_country, c.hq_country_code,
       c.kind, c.industry, c.source, c.updated_at
FROM companies c
JOIN company_countries cc ON cc.company_id = c.id
WHERE cc.country_key = ?
  AND (lower(c.name) LIKE ? OR lower(c.website) LIKE ? OR lower(c.board) LIKE ?
       OR lower(c.ats) LIKE ? OR lower(c.board_url) LIKE ?)
ORDER BY c.name COLLATE NOCASE
LIMIT ?`, key, like, like, like, like, like, limit)
		}
	} else if q == "" {
		rows, err = s.db.Query(`
SELECT id, name, website, ats, board, board_url,
       hire_countries, hire_country_codes, hq_country, hq_country_code,
       kind, industry, source, updated_at
FROM companies
ORDER BY name COLLATE NOCASE
LIMIT ?`, limit)
	} else {
		rows, err = s.db.Query(`
SELECT id, name, website, ats, board, board_url,
       hire_countries, hire_country_codes, hq_country, hq_country_code,
       kind, industry, source, updated_at
FROM companies
WHERE lower(name) LIKE ? OR lower(website) LIKE ? OR lower(board) LIKE ?
   OR lower(ats) LIKE ? OR lower(board_url) LIKE ?
ORDER BY name COLLATE NOCASE
LIMIT ?`, like, like, like, like, like, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list, err := scanCompanies(rows)
	if err != nil {
		return nil, err
	}
	return dedupeCompaniesByName(list), nil
}

// dedupeCompaniesByName keeps one row per company name (case-insensitive),
// preferring the row that has a known ATS board.
func dedupeCompaniesByName(in []Company) []Company {
	best := map[string]int{} // lower name → index in out
	var out []Company
	for _, c := range in {
		k := strings.ToLower(strings.TrimSpace(c.Name))
		if k == "" {
			continue
		}
		if i, ok := best[k]; ok {
			prev := out[i]
			if prev.ATS == "" && c.ATS != "" {
				out[i] = c
			} else if prev.BoardURL == "" && c.BoardURL != "" {
				out[i] = c
			}
			continue
		}
		best[k] = len(out)
		out = append(out, c)
	}
	return out
}
