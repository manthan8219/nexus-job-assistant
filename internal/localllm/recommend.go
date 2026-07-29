package localllm

import "sort"

// Recommendation is a catalog model annotated for the current machine.
type Recommendation struct {
	Model
	Fits      bool
	Installed bool
	Best      bool // top fit for this machine
}

// Recommend returns catalog models sorted best-first for the machine.
// Only models that fit in RAM are marked Fits=true; the list still includes
// a few that don't (dimmed in UI) so the user sees what's available later.
func Recommend(machine Machine, installed []string) []Recommendation {
	inst := map[string]bool{}
	for _, n := range installed {
		inst[n] = true
		// also match bare name without tag
		if i := indexByte(n, ':'); i >= 0 {
			inst[n[:i]] = true
		}
	}

	// Usable RAM budget — leave headroom for OS + Nexus.
	budget := machine.RAMGB - 2
	if budget < 4 {
		budget = machine.RAMGB
	}

	out := make([]Recommendation, 0, len(Catalog))
	for _, m := range Catalog {
		r := Recommendation{
			Model:     m,
			Fits:      m.MinRAMGB <= budget,
			Installed: inst[m.Name] || inst[stripTag(m.Name)],
		}
		out = append(out, r)
	}

	sort.SliceStable(out, func(i, j int) bool {
		// Fitting models first, then by quality desc.
		if out[i].Fits != out[j].Fits {
			return out[i].Fits
		}
		return out[i].Quality > out[j].Quality
	})

	// Mark the single best fitting model.
	for i := range out {
		if out[i].Fits {
			out[i].Best = true
			break
		}
	}
	return out
}

// TopFits returns up to n fitting recommendations (already sorted).
func TopFits(recs []Recommendation, n int) []Recommendation {
	var out []Recommendation
	for _, r := range recs {
		if !r.Fits {
			continue
		}
		out = append(out, r)
		if len(out) >= n {
			break
		}
	}
	return out
}

func stripTag(name string) string {
	if i := indexByte(name, ':'); i >= 0 {
		return name[:i]
	}
	return name
}

func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}
