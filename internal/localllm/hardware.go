package localllm

import (
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// Machine describes local hardware used for model recommendations.
type Machine struct {
	RAMGB     int
	CPU       string
	GOOS      string
	GOARCH    string
	GPUName   string // empty when no dedicated GPU was detected
	GPUVRAMGB int    // 0 when unknown or no dedicated GPU
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
	m.GPUName, m.GPUVRAMGB = probeGPU()
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
	case "windows":
		out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command",
			"(Get-CimInstance Win32_ComputerSystem).TotalPhysicalMemory").Output()
		if err != nil {
			return 0
		}
		bytes, err := strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
		if err != nil || bytes == 0 {
			return 0
		}
		return int(bytes / (1024 * 1024 * 1024))
	default:
		return 0
	}
}

// probeGPU returns the dedicated GPU name and VRAM in GB, best-effort.
// Returns ("", 0) when no dedicated GPU is found or detection fails.
func probeGPU() (string, int) {
	// nvidia-smi is the reliable source on both Windows and Linux when an
	// NVIDIA GPU is present — Windows' WMI AdapterRAM field is unreliable
	// (commonly capped at 4GB by a known 32-bit registry limit).
	out, err := exec.Command("nvidia-smi", "--query-gpu=name,memory.total", "--format=csv,noheader,nounits").Output()
	if err == nil {
		line := strings.TrimSpace(strings.Split(string(out), "\n")[0])
		parts := strings.Split(line, ",")
		if len(parts) == 2 {
			name := strings.TrimSpace(parts[0])
			miB, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
			if name != "" && err == nil && miB > 0 {
				return name, int(miB / 1024)
			}
		}
	}

	if runtime.GOOS == "windows" {
		// Name-only fallback (no NVIDIA tooling, e.g. AMD/Intel GPUs) —
		// AdapterRAM is intentionally not used, see comment above.
		out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command",
			"(Get-CimInstance Win32_VideoController | Select-Object -First 1).Name").Output()
		if err == nil {
			name := strings.TrimSpace(string(out))
			if name != "" {
				return name, 0
			}
		}
	}
	return "", 0
}

func probeCPU() string {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output()
		if err == nil {
			return strings.TrimSpace(string(out))
		}
	case "windows":
		out, err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command",
			"(Get-CimInstance Win32_Processor).Name").Output()
		if err == nil && strings.TrimSpace(string(out)) != "" {
			return strings.TrimSpace(string(out))
		}
	}
	return runtime.GOARCH
}
