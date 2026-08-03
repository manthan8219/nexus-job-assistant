package api

import "github.com/manthan8219/nexus-job-assistant/internal/config"

// configToNexusConfig converts a backend config.Config to the frontend shape.
func configToNexusConfig(cfg *config.Config) *NexusConfig {
	return &NexusConfig{
		FirstName:              cfg.FirstName,
		LastName:               cfg.LastName,
		Email:                  cfg.Email,
		Phone:                  cfg.Phone,
		LinkedinID:             cfg.LinkedInID,
		ResumePath:             cfg.ResumePath,
		City:                   cfg.City,
		YearsOfExperience:      cfg.YearsOfExperience,
		Skills:                 cfg.Skills,
		TargetJobTitles:        cfg.TargetJobTitles,
		JobIntent:              cfg.JobIntent,
		WorkType:               cfg.WorkType,
		TargetLocations:        cfg.TargetLocations,
		Currency:               cfg.Currency,
		MinSalary:              cfg.MinSalary,
		ProviderKeys:           cfg.ProviderKeys,
		AIAssist:               cfg.AIAssist,
		AIProvider:             cfg.AIProvider,
		AnthropicKey:           cfg.AnthropicKey,
		OpenAIKey:              cfg.OpenAIKey,
		GoogleKey:              cfg.GoogleKey,
		DeepSeekKey:            cfg.DeepSeekKey,
		GroqKey:                cfg.GroqKey,
		MistralKey:             cfg.MistralKey,
		TogetherKey:            cfg.TogetherKey,
		OpenRouterKey:          cfg.OpenRouterKey,
		XAIKey:                 cfg.XAIKey,
		LocalLLMURL:            cfg.LocalLLMURL,
		LocalLLMModel:          cfg.LocalLLMModel,
		ApplyConsent:           cfg.ApplyConsent,
		ApplyConsentAt:         cfg.ApplyConsentAt,
		MaxAppsPerRun:          cfg.MaxAppsPerRun,
		MaxAppsPerDay:          cfg.MaxAppsPerDay,
		ApplyDelaySec:          cfg.ApplyDelaySec,
		MinFitScore:            cfg.MinFitScore,
		CompanyBlocklist:       cfg.CompanyBlocklist,
		WorkAuth:               cfg.WorkAuth,
		NoticePeriodDays:       parseInt(cfg.NoticePeriodDays),
		OfficeDaysPerWeek:      parseInt(cfg.OfficeDaysPerWeek),
		CoverLetterMode:        cfg.CoverLetterMode,
		CoverLetterText:        cfg.CoverLetterText,
		OutreachConsent:        cfg.OutreachConsent,
		MaxEmailsPerDay:        cfg.MaxEmailsPerDay,
		MaxLinkedInPerDay:      cfg.MaxLinkedInPerDay,
		OutreachMode:           cfg.OutreachMode,
		OutreachAutoQueue:      cfg.OutreachAutoQueue,
		OutreachAICompose:      cfg.OutreachAICompose,
		OutreachAIReview:       cfg.OutreachAIReview,
		GmailAppPassword:       cfg.GmailAppPassword,
		HunterKey:              cfg.HunterKey,
		ApolloKey:              cfg.ApolloKey,
		GmailOAuthClientID:     cfg.GmailOAuthClientID,
		GmailOAuthClientSecret: cfg.GmailOAuthClientSecret,
		GmailOAuthRefreshToken: cfg.GmailOAuthRefreshToken,
		DiscordWebhookURL:      cfg.DiscordWebhookURL,
		TelegramBotToken:       cfg.TelegramBotToken,
		TelegramChatID:         cfg.TelegramChatID,
		NotifyChannels:         cfg.NotifyChannels,
		TailorPerJob:           cfg.TailorPerJob,
		TailorMaxRounds:        cfg.TailorMaxRounds,
		ScraperTargets:         cfg.ScraperTargets,
		DailyRunEnabled:        cfg.DailyRunEnabled,
		DailyRunAt:             cfg.DailyRunAt,
		EmailNotifications:     cfg.EmailNotifications,
	}
}

// applyNexusConfig copies frontend fields back into the backend config.
func applyNexusConfig(cfg *config.Config, n *NexusConfig) {
	cfg.FirstName = n.FirstName
	cfg.LastName = n.LastName
	cfg.Email = n.Email
	cfg.Phone = n.Phone
	cfg.LinkedInID = n.LinkedinID
	cfg.ResumePath = n.ResumePath
	cfg.City = n.City
	cfg.YearsOfExperience = n.YearsOfExperience
	cfg.Skills = n.Skills
	cfg.TargetJobTitles = n.TargetJobTitles
	cfg.JobIntent = n.JobIntent
	cfg.WorkType = n.WorkType
	cfg.TargetLocations = n.TargetLocations
	cfg.Currency = n.Currency
	cfg.MinSalary = n.MinSalary
	cfg.ProviderKeys = n.ProviderKeys
	cfg.AIAssist = n.AIAssist
	cfg.AIProvider = n.AIProvider
	cfg.AnthropicKey = n.AnthropicKey
	cfg.OpenAIKey = n.OpenAIKey
	cfg.GoogleKey = n.GoogleKey
	cfg.DeepSeekKey = n.DeepSeekKey
	cfg.GroqKey = n.GroqKey
	cfg.MistralKey = n.MistralKey
	cfg.TogetherKey = n.TogetherKey
	cfg.OpenRouterKey = n.OpenRouterKey
	cfg.XAIKey = n.XAIKey
	cfg.LocalLLMURL = n.LocalLLMURL
	cfg.LocalLLMModel = n.LocalLLMModel
	cfg.ApplyConsent = n.ApplyConsent
	cfg.ApplyConsentAt = n.ApplyConsentAt
	cfg.MaxAppsPerRun = n.MaxAppsPerRun
	cfg.MaxAppsPerDay = n.MaxAppsPerDay
	cfg.ApplyDelaySec = n.ApplyDelaySec
	cfg.MinFitScore = n.MinFitScore
	cfg.CompanyBlocklist = n.CompanyBlocklist
	cfg.WorkAuth = n.WorkAuth
	cfg.NoticePeriodDays = itoa(n.NoticePeriodDays)
	cfg.OfficeDaysPerWeek = itoa(n.OfficeDaysPerWeek)
	cfg.CoverLetterMode = n.CoverLetterMode
	cfg.CoverLetterText = n.CoverLetterText
	cfg.OutreachConsent = n.OutreachConsent
	cfg.MaxEmailsPerDay = n.MaxEmailsPerDay
	cfg.MaxLinkedInPerDay = n.MaxLinkedInPerDay
	cfg.OutreachMode = n.OutreachMode
	cfg.OutreachAutoQueue = n.OutreachAutoQueue
	cfg.OutreachAICompose = n.OutreachAICompose
	cfg.OutreachAIReview = n.OutreachAIReview
	cfg.GmailAppPassword = n.GmailAppPassword
	cfg.HunterKey = n.HunterKey
	cfg.ApolloKey = n.ApolloKey
	cfg.GmailOAuthClientID = n.GmailOAuthClientID
	cfg.GmailOAuthClientSecret = n.GmailOAuthClientSecret
	cfg.GmailOAuthRefreshToken = n.GmailOAuthRefreshToken
	cfg.DiscordWebhookURL = n.DiscordWebhookURL
	cfg.TelegramBotToken = n.TelegramBotToken
	cfg.TelegramChatID = n.TelegramChatID
	cfg.NotifyChannels = n.NotifyChannels
	cfg.TailorPerJob = n.TailorPerJob
	cfg.TailorMaxRounds = n.TailorMaxRounds
	cfg.ScraperTargets = n.ScraperTargets
	cfg.DailyRunEnabled = n.DailyRunEnabled
	cfg.DailyRunAt = n.DailyRunAt
	cfg.EmailNotifications = n.EmailNotifications
}

// parseInt converts a string to int, returning 0 on error.
func parseInt(s string) int {
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// itoa converts an int to a string.
func itoa(n int) string {
	if n == 0 {
		return ""
	}
	buf := make([]byte, 0, 10)
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	return string(buf)
}
