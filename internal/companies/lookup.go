package companies

import "fmt"

// FindByCountry is the main entry: open default DB and return companies for a country.
// country may be ISO2 ("IN") or name ("India").
func FindByCountry(country string) ([]Company, error) {
	db, err := OpenDefault()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	n, err := db.Count()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, fmt.Errorf("company database is empty — run: go run ./cmd/companies-seed")
	}
	return db.FindByCountry(country)
}

// FindByCountryWithATS filters FindByCountry to rows that have a known public ATS board.
func FindByCountryWithATS(country string) ([]Company, error) {
	all, err := FindByCountry(country)
	if err != nil {
		return nil, err
	}
	var out []Company
	for _, c := range all {
		if HasATSBoard(c) {
			c.Board = BoardToken(c)
			out = append(out, c)
		}
	}
	return out, nil
}
