package resume

import (
	"reflect"
	"testing"
)

func TestExtractContact(t *testing.T) {
	const resumeText = `Manthan Bhatia
Senior Backend Engineer
manthan.bhatia@example.com
+1 (555) 010-1234
linkedin.com/in/manthan-bhatia
8+ years building distributed systems with Go and Kubernetes.`

	tests := []struct {
		name    string
		text    string
		aiYears int
		want    Contact
	}{
		{
			name: "full contact block",
			text: resumeText,
			want: Contact{
				FirstName: "Manthan",
				LastName:  "Bhatia",
				Email:     "manthan.bhatia@example.com",
				Phone:     "+1 (555) 010-1234",
				LinkedIn:  "manthan-bhatia",
				Years:     "8",
			},
		},
		{
			name:    "AI years estimate wins over regex",
			text:    "3 years experience\nmanthan@example.com",
			aiYears: 7,
			want:    Contact{Email: "manthan@example.com", Years: "7"},
		},
		{
			name: "picks the first plausible line as the name",
			text: "John Doe\nSENIOR BACKEND ENGINEER\njohn@example.com",
			want: Contact{FirstName: "John", LastName: "Doe", Email: "john@example.com"},
		},
		{
			name: "email does not swallow glued PDF text",
			text: "Email: manthanbhatia367@gmail.comGitHub\n+91 8219751407",
			want: Contact{
				Email: "manthanbhatia367@gmail.com",
				Phone: "+91 8219751407",
			},
		},
		{
			name: "no contact info",
			text: "just some resume text without details",
			want: Contact{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractContact(tt.text, tt.aiYears)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ExtractContact() = %+v; want %+v", got, tt.want)
			}
		})
	}
}
