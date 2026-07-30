package smartrecruiters

// API response types for SmartRecruiters public job board API

type srResponse struct {
	Content []srPosting `json:"content"`
}

type srPosting struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	UUID             string     `json:"uuid"`
	Company          srCompany  `json:"company"`
	Location         srLocation `json:"location"`
	TypeOfEmployment srTyp      `json:"typeOfEmployment"`
	ReleasedDate     string     `json:"releasedDate"`
}

type srCompany struct {
	Identifier string `json:"identifier"`
	Name       string `json:"name"`
}

type srLocation struct {
	City         string `json:"city"`
	Country      string `json:"country"`
	Remote       bool   `json:"remote"`
	FullLocation string `json:"fullLocation"`
}

type srTyp struct {
	ID string `json:"id"`
}

type srCompanyEntry struct {
	Name       string `json:"name"`
	Identifier string `json:"identifier"` // SmartRecruiters company ID
}
