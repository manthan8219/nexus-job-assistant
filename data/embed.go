package data

import _ "embed"

//go:embed companies.json
var CompaniesJSON []byte

//go:embed ashby_companies.json
var AshbyCompaniesJSON []byte

//go:embed smartrecruiters_companies.json
var SmartRecruitersCompaniesJSON []byte

//go:embed lever_companies.json
var LeverCompaniesJSON []byte

//go:embed workable_companies.json
var WorkableCompaniesJSON []byte

//go:embed workday_companies.json
var WorkdayCompaniesJSON []byte

//go:embed bamboohr_companies.json
var BambooHRCompaniesJSON []byte

//go:embed recruitee_companies.json
var RecruiteeCompaniesJSON []byte

//go:embed breezy_companies.json
var BreezyCompaniesJSON []byte

//go:embed pinpoint_companies.json
var PinpointCompaniesJSON []byte

//go:embed jobvite_companies.json
var JobviteCompaniesJSON []byte

//go:embed teamtailor_companies.json
var TeamtailorCompaniesJSON []byte

//go:embed personio_companies.json
var PersonioCompaniesJSON []byte

// CitiesIndexGZ is a gzipped slim city→country index derived from
// dr5hn/countries-states-cities-database (Open Database License).
// Rebuild with: python3 scripts/build_cities_index.py
//
//go:embed cities_index.json.gz
var CitiesIndexGZ []byte

//go:embed india_employers.json
var IndiaEmployersJSON []byte
