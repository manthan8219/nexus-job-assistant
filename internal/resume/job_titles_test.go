package resume

import "testing"

func TestParseJobTitles(t *testing.T) {
	titles, err := parseJobTitles(`{"titles":["Backend Engineer","  Senior Go Engineer ","Backend Engineer"]}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(titles) != 2 {
		t.Fatalf("got %#v", titles)
	}
}
