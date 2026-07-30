package engine

// Package engine — engine_config.go
// Config → domain conversions: building the provider.Profile and
// provider.SearchCriteria the search/apply loop consumes, and the human-like
// inter-apply delay.

import (
	"math/rand"
	"strconv"
	"time"

	"github.com/manthan8219/nexus-job-assistant/internal/config"
	"github.com/manthan8219/nexus-job-assistant/internal/geo"
	"github.com/manthan8219/nexus-job-assistant/internal/provider"
	"github.com/manthan8219/nexus-job-assistant/internal/provider/greenhouse"
)

// profileFromConfig converts config to a provider.Profile.
func profileFromConfig(cfg *config.Config) provider.Profile {
	return provider.Profile{
		FirstName:  cfg.FirstName,
		LastName:   cfg.LastName,
		Email:      cfg.Email,
		Phone:      cfg.Phone,
		ResumePath: cfg.ResumePath,
		LinkedInID: cfg.LinkedInID,
		City:       cfg.City,
		YearsExp:   cfg.YearsOfExperience,
		MinSalary:  cfg.MinSalary,
	}
}

// criteriaFromConfig converts config to a provider.SearchCriteria.
func criteriaFromConfig(cfg *config.Config) provider.SearchCriteria {
	salary, _ := strconv.Atoi(cfg.MinSalary)
	return provider.SearchCriteria{
		Titles:    greenhouse.ParseTitles(cfg.TargetJobTitles),
		Locations: geo.ExpandLocations(geo.ParseLocationTags(cfg.TargetLocations)),
		WorkType:  cfg.WorkType,
		MinSalary: salary,
	}
}

// humanDelay returns a random delay between minSecs and minSecs+12 seconds.
func humanDelay(minSecs int) time.Duration {
	return time.Duration(minSecs+rand.Intn(12)) * time.Second
}
