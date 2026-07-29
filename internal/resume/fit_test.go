package resume

import "testing"

func TestParseFit(t *testing.T) {
	r, err := parseFit(`{"score":72,"summary":"Strong Go match; light cloud experience."}`)
	if err != nil {
		t.Fatal(err)
	}
	if r.Score != 72 || r.Summary == "" {
		t.Fatalf("%+v", r)
	}
}
