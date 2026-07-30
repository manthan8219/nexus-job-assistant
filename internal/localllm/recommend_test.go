package localllm

import "testing"

func TestRecommend_FiltersByRAM(t *testing.T) {
	recs := Recommend(Machine{RAMGB: 8}, nil)
	if len(recs) == 0 {
		t.Fatal("expected recommendations")
	}
	var foundBest bool
	for _, r := range recs {
		if r.Best {
			foundBest = true
			if !r.Fits {
				t.Fatal("best must fit")
			}
			if r.MinRAMGB > 6 { // budget = 8-2 = 6... wait 8-2=6, llama3.2 needs 8
				// budget 6 means MinRAMGB<=6 fits
			}
		}
		if r.Fits && r.MinRAMGB > 6 {
			t.Errorf("%s marked Fits but MinRAMGB=%d budget=6", r.Name, r.MinRAMGB)
		}
	}
	if !foundBest {
		t.Fatal("expected a Best recommendation")
	}
}

func TestRecommend_MarksInstalled(t *testing.T) {
	recs := Recommend(Machine{RAMGB: 16}, []string{"llama3.2:latest"})
	found := false
	for _, r := range recs {
		if r.Name == "llama3.2" && r.Installed {
			found = true
		}
	}
	if !found {
		t.Fatal("expected llama3.2 marked installed")
	}
}

func TestTopFits(t *testing.T) {
	recs := Recommend(Machine{RAMGB: 16}, nil)
	top := TopFits(recs, 5)
	if len(top) == 0 || len(top) > 5 {
		t.Fatalf("got %d", len(top))
	}
	for _, r := range top {
		if !r.Fits {
			t.Fatalf("%s should fit", r.Name)
		}
	}
}
