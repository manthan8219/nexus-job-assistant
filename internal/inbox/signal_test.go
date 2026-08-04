package inbox

import "testing"

func TestClassify(t *testing.T) {
	tests := []struct {
		name    string
		subject string
		body    string
		want    Signal
	}{
		{"interview subject", "Interview invitation - Backend Engineer", "", SignalInterview},
		{"interview body only", "General", "We would love to schedule a call with you.", SignalInterview},
		{"rejection", "Your application status", "We regret to inform you that we are not moving forward.", SignalRejection},
		{"offer", "Congratulations", "We are pleased to offer you the position.", SignalOffer},
		{"assessment", "Coding assessment", "Please complete the take-home coding assessment.", SignalAssessment},
		{"recruiter", "Opportunity at Acme", "We found your profile and think you'd be a great fit.", SignalRecruiter},
		{"application", "Application received", "Thank you for applying to Acme.", SignalApplication},
		{"none", "Weekly newsletter", "Your weekly digest is ready.", SignalNone},
		{"rejection wins over interview", "After your interview", "Unfortunately we decided not to proceed.", SignalRejection},
		{"offer-an-interview is interview", "We'd like to offer you an interview", "", SignalInterview},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := Classify(tt.subject, tt.body)
			if got != tt.want {
				t.Errorf("Classify(%q, %q) = %v; want %v", tt.subject, tt.body, got, tt.want)
			}
		})
	}
}
