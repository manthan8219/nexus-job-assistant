package localllm

import (
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// Machine describes local hardware used for model recommendations.
type Machine struct {
	RAMGB   int
	CPU     string
	GOOS    string
	GOARCH  string
}

// ProbeMachine returns a best-effort hardware snapshot.
func ProbeMachine() Machine {
	m := Machine{
		GOOS:   runtime.GOOS,
		GOARCH: runtime.GOARCH,
		RAMGB:  8, // safe default if probe fails
	}
	if gb := probeRAMGB(); gb > 0 {
		m.RAMGB = gb
	}
	m.CPU = probeCPU()
	return m
}

func probeRAMGB() int {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("sysctl", "-n", "hw.memsize").Output()
		if err != nil {
			return 0
		}
		bytes, err := strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
		if err != nil || bytes == 0 {
			return 0
		}
		return int(bytes / (1024 * 1024 * 1024))
	case "linux":
		out, err := exec.Command("grep", "MemTotal:", "/proc/meminfo").Output()
		if err != nil {
			return 0
		}
		// MemTotal:       16384000 kB
		fields := strings.Fields(string(out))
		if len(fields) < 2 {
			return 0
		}
		kb, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return 0
		}
		return int(kb / (1024 * 1024))
	default:
		return 0
	}
}

func probeCPU() string {
	if runtime.GOOS == "darwin" {
		out, err := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output()
		if err == nil {
			return strings.TrimSpace(string(out))
		}
	}
	return runtime.GOARCH
}
