package engine

// Package engine — engine_new.go
// Engine construction: New wires up every registered provider and builds the
// Engine with its channels and notifier. Provider registration is the single
// discovery point (see AGENTS.md §8).

import (
	"fmt"
	"time"

	"github.com/manthan8219/nexus-job-assistant/data"
	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/provider"
	"github.com/manthan8219/nexus-job-assistant/internal/provider/adzuna"
	"github.com/manthan8219/nexus-job-assistant/internal/provider/arbeitnow"
	"github.com/manthan8219/nexus-job-assistant/internal/provider/ashby"
	"github.com/manthan8219/nexus-job-assistant/internal/provider/bamboohr"
	"github.com/manthan8219/nexus-job-assistant/internal/provider/breezy"
	"github.com/manthan8219/nexus-job-assistant/internal/provider/careerscraper"
	"github.com/manthan8219/nexus-job-assistant/internal/provider/fourday"
	"github.com/manthan8219/nexus-job-assistant/internal/provider/getonbrd"
	"github.com/manthan8219/nexus-job-assistant/internal/provider/greenhouse"
	"github.com/manthan8219/nexus-job-assistant/internal/provider/hackernews"
	"github.com/manthan8219/nexus-job-assistant/internal/provider/himalayas"
	"github.com/manthan8219/nexus-job-assistant/internal/provider/jobicy"
	"github.com/manthan8219/nexus-job-assistant/internal/provider/jobspresso"
	"github.com/manthan8219/nexus-job-assistant/internal/provider/jobvite"
	"github.com/manthan8219/nexus-job-assistant/internal/provider/justjoin"
	"github.com/manthan8219/nexus-job-assistant/internal/provider/lever"
	"github.com/manthan8219/nexus-job-assistant/internal/provider/linkedin"
	"github.com/manthan8219/nexus-job-assistant/internal/provider/nodesk"
	"github.com/manthan8219/nexus-job-assistant/internal/provider/nofluffjobs"
	"github.com/manthan8219/nexus-job-assistant/internal/provider/personio"
	"github.com/manthan8219/nexus-job-assistant/internal/provider/pinpoint"
	"github.com/manthan8219/nexus-job-assistant/internal/provider/recruitee"
	"github.com/manthan8219/nexus-job-assistant/internal/provider/remoteok"
	"github.com/manthan8219/nexus-job-assistant/internal/provider/remotive"
	"github.com/manthan8219/nexus-job-assistant/internal/provider/smartrecruiters"
	"github.com/manthan8219/nexus-job-assistant/internal/provider/teamtailor"
	"github.com/manthan8219/nexus-job-assistant/internal/provider/thehub"
	"github.com/manthan8219/nexus-job-assistant/internal/provider/themuse"
	"github.com/manthan8219/nexus-job-assistant/internal/provider/weworkremotely"
	"github.com/manthan8219/nexus-job-assistant/internal/provider/workable"
	"github.com/manthan8219/nexus-job-assistant/internal/provider/workday"
	"github.com/manthan8219/nexus-job-assistant/internal/provider/workingnomads"
	"github.com/manthan8219/nexus-job-assistant/internal/provider/wttj"
	scr "github.com/manthan8219/nexus-job-assistant/internal/scraper"
	"github.com/manthan8219/nexus-job-assistant/internal/store"
)

// New builds an Engine. If companiesPath is non-empty it overrides the embedded companies list.
func New(cfg *config.Config, st *store.Store, companiesPath string) (*Engine, error) {
	var providers []provider.Provider

	// Greenhouse
	var gh *greenhouse.Client
	var err error
	if companiesPath != "" {
		gh, err = greenhouse.NewFromFile(companiesPath)
	} else {
		gh, err = greenhouse.New(data.CompaniesJSON)
	}
	if err != nil {
		return nil, fmt.Errorf("greenhouse init: %w", err)
	}
	providers = append(providers, gh)

	// Ashby — always active, no user key required
	ab, err := ashby.New(data.AshbyCompaniesJSON)
	if err != nil {
	} else {
		providers = append(providers, ab)
	}

	// SmartRecruiters — always active, no user key required
	sr, err := smartrecruiters.New(data.SmartRecruitersCompaniesJSON)
	if err != nil {
	} else {
		providers = append(providers, sr)
	}

	lv, err := lever.New(data.LeverCompaniesJSON)
	if err != nil {
	} else {
		providers = append(providers, lv)
	}

	wk, err := workable.New(data.WorkableCompaniesJSON)
	if err != nil {
	} else {
		providers = append(providers, wk)
	}

	// Board-wide aggregators — no company list or API key required.
	providers = append(providers, remoteok.New())
	providers = append(providers, remotive.New())
	providers = append(providers, arbeitnow.New())
	providers = append(providers, jobicy.New())
	providers = append(providers, hackernews.New())

	// Workday — per-company ATS, sourced from data/workday_companies.json
	wd, err := workday.New(data.WorkdayCompaniesJSON)
	if err != nil {
	} else {
		providers = append(providers, wd)
	}

	// Board-wide aggregators (Group A: JSON)
	providers = append(providers, himalayas.New())
	providers = append(providers, fourday.New())
	providers = append(providers, workingnomads.New())
	providers = append(providers, themuse.New())
	providers = append(providers, thehub.New())
	providers = append(providers, getonbrd.New())
	providers = append(providers, nofluffjobs.New())
	providers = append(providers, justjoin.New())
	providers = append(providers, wttj.New())

	// Board-wide aggregators (Group B: RSS/XML)
	providers = append(providers, weworkremotely.New())
	providers = append(providers, jobspresso.New())
	providers = append(providers, nodesk.New())

	// Key-gated job finders — only registered when the API key is configured.
	if id, key := cfg.ProviderKeys["adzuna_id"], cfg.ProviderKeys["adzuna_key"]; id != "" && key != "" {
		providers = append(providers, adzuna.New(id, key, ""))
	}

	// Per-company ATSes (Group C)
	bhr, err := bamboohr.New(data.BambooHRCompaniesJSON)
	if err != nil {
	} else {
		providers = append(providers, bhr)
	}

	rct, err := recruitee.New(data.RecruiteeCompaniesJSON)
	if err != nil {
	} else {
		providers = append(providers, rct)
	}

	bzy, err := breezy.New(data.BreezyCompaniesJSON)
	if err != nil {
	} else {
		providers = append(providers, bzy)
	}

	ppt, err := pinpoint.New(data.PinpointCompaniesJSON)
	if err != nil {
	} else {
		providers = append(providers, ppt)
	}

	// TODO(india-providers): scaffolded but not yet implemented — see
	// internal/provider/{instahyre,hirist,cutshort}/provider.go
	// for status notes. Wire in with providers = append(providers, X.New())
	// once each Search() is implemented against a confirmed endpoint.

	// TODO(workatastartup): scaffolded but not yet implemented — see
	// internal/provider/workatastartup/provider.go. Requires a logged-in
	// session; unofficial endpoint, keep request volume low once built.

	jv, err := jobvite.New(data.JobviteCompaniesJSON)
	if err != nil {
	} else {
		providers = append(providers, jv)
	}

	tt, err := teamtailor.New(data.TeamtailorCompaniesJSON)
	if err != nil {
	} else {
		providers = append(providers, tt)
	}

	ps, err := personio.New(data.PersonioCompaniesJSON)
	if err != nil {
	} else {
		providers = append(providers, ps)
	}

	// Career page scraper — only added when the Python service is installed and running.
	// It auto-discovers /jobs and /careers pages for companies without a known ATS.
	if scr.Installed() {
		if !scr.Running() {
			ollamaURL := cfg.LocalLLMURL
			if ollamaURL == "" {
				ollamaURL = "http://localhost:11434"
			}
			_ = scr.Start(cfg.LocalLLMModel, ollamaURL)
			_ = scr.WaitReady(15 * time.Second)
		}
		if scr.Running() {
			providers = append(providers, careerscraper.New(nil, cfg.LocalLLMModel, cfg.LocalLLMURL))
			providers = append(providers, linkedin.New(3))
		}
	}

	return &Engine{
		cfg:        cfg,
		store:      st,
		providers:  providers,
		Notifier:   notifierFromCfg(cfg),
		LogCh:      make(chan string, 100),
		ResultCh:   make(chan Result, 500),
		ProgressCh: make(chan ProviderProgress, 100),
		MaxPerRun:  10,
		MinDelay:   8,
	}, nil
}
