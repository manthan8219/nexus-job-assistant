package api

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/localllm"
)

// LLMStatus contains info about the local LLM runtime.
type LLMStatus struct {
	Reachable bool         `json:"reachable"`
	Installed []string     `json:"installed,omitempty"`
	Machine   *MachineInfo `json:"machine,omitempty"`
	Models    []ModelRec   `json:"models,omitempty"`
	Err       string       `json:"err,omitempty"`
}

// MachineInfo holds hardware info for model recommendations.
type MachineInfo struct {
	RAMGB     int    `json:"ramGb"`
	CPU       string `json:"cpu"`
	GOOS      string `json:"goos"`
	GOARCH    string `json:"goarch"`
	GPUName   string `json:"gpuName,omitempty"`
	GPUVRAMGB int    `json:"gpuVramGb,omitempty"`
}

// ModelRec is a recommended model.
type ModelRec struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	MinRAMGB    int    `json:"minRamGb"`
	Fits        bool   `json:"fits"`
	Installed   bool   `json:"installed"`
	Best        bool   `json:"best,omitempty"`
	Notes       string `json:"notes,omitempty"`
}

// PullProgress tracks an ongoing model download.
type PullProgress struct {
	Model     string `json:"model"`
	Status    string `json:"status"` // "starting" | "downloading" | "complete" | "error"
	Message   string `json:"message"`
	Completed int64  `json:"completed,omitempty"`
	Total     int64  `json:"total,omitempty"`
	Error     string `json:"error,omitempty"`
}

// pullProgressMap stores ongoing pull progress per model name.
var pullProgressMap sync.Map

// llmBaseURLFor returns the Ollama base URL from the request's config or the
// localhost default.
func (s *Server) llmBaseURLFor(r *http.Request) string {
	if cfg := s.cfgFor(r); cfg != nil && cfg.LocalLLMURL != "" {
		return cfg.LocalLLMURL
	}
	return "http://localhost:11434"
}

// handleLLMStatus returns the status of the local LLM runtime (Ollama).
func (s *Server) handleLLMStatus(w http.ResponseWriter, r *http.Request) {
	baseURL := s.llmBaseURLFor(r)
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()

	client := localllm.NewClient(baseURL)
	status := LLMStatus{}

	if err := client.Ping(ctx); err != nil {
		status.Reachable = false
		status.Err = err.Error()
		writeJSON(w, http.StatusOK, status)
		return
	}

	status.Reachable = true
	installed, err := client.ListInstalled(ctx)
	if err == nil {
		status.Installed = installed
	}

	machine := localllm.ProbeMachine()
	status.Machine = &MachineInfo{
		RAMGB:     machine.RAMGB,
		CPU:       machine.CPU,
		GOOS:      machine.GOOS,
		GOARCH:    machine.GOARCH,
		GPUName:   machine.GPUName,
		GPUVRAMGB: machine.GPUVRAMGB,
	}

	recs := localllm.Recommend(machine, installed)
	for _, r := range recs {
		status.Models = append(status.Models, ModelRec{
			Name:        r.Name,
			DisplayName: r.DisplayName,
			MinRAMGB:    r.MinRAMGB,
			Fits:        r.Fits,
			Installed:   r.Installed,
			Best:        r.Best,
			Notes:       r.Notes,
		})
	}

	writeJSON(w, http.StatusOK, status)
}

// handleLLMPull starts a model download in the background and returns immediately.
func (s *Server) handleLLMPull(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Model string `json:"model"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if body.Model == "" {
		writeError(w, http.StatusBadRequest, "model name is required")
		return
	}

	// Check if already pulling
	key := body.Model
	if _, loaded := pullProgressMap.Load(key); loaded {
		writeError(w, http.StatusConflict, "already pulling this model")
		return
	}

	// Initialize progress
	pp := &PullProgress{Model: key, Status: "starting", Message: "Starting download..."}
	pullProgressMap.Store(key, pp)

	baseURL := s.llmBaseURLFor(r)
	client := localllm.NewClient(baseURL)

	// Start pull in background
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				pp.Status = "error"
				pp.Error = "pull crashed"
				pullProgressMap.Store(key, pp)
			}
		}()

		ctx := context.Background()
		if err := client.Ping(ctx); err != nil {
			pp.Status = "error"
			pp.Error = "Ollama not reachable: " + err.Error()
			pullProgressMap.Store(key, pp)
			return
		}

		err := client.Pull(ctx, key, func(status string, completed, total int64) {
			pp.Status = "downloading"
			pp.Message = status
			pp.Completed = completed
			pp.Total = total
			pullProgressMap.Store(key, pp)
		})

		if err != nil {
			pp.Status = "error"
			pp.Error = err.Error()
		} else {
			pp.Status = "complete"
			pp.Message = "Download complete"
			pp.Completed = pp.Total
		}
		pullProgressMap.Store(key, pp)

		// Auto-cleanup after 30 seconds
		time.AfterFunc(30*time.Second, func() {
			pullProgressMap.Delete(key)
		})
	}()

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":    true,
		"model": key,
	})
}

// handleLLMPullStatus returns the current progress of a model pull.
func (s *Server) handleLLMPullStatus(w http.ResponseWriter, r *http.Request) {
	model := r.PathValue("model")
	if model == "" {
		writeError(w, http.StatusBadRequest, "model name is required")
		return
	}

	val, ok := pullProgressMap.Load(model)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{
			"model":  model,
			"status": "unknown",
		})
		return
	}

	pp := val.(*PullProgress)
	writeJSON(w, http.StatusOK, pp)
}
