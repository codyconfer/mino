package github

import "testing"

func TestNormalizeAPIURL(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{"empty", "", "", false},
		{"https default", "https://api.github.com", "https://api.github.com", false},
		{"https trailing slash", "https://ghe.example.com/api/v3/", "https://ghe.example.com/api/v3", false},
		{"http rejected", "http://evil.example/api/v3", "", true},
		{"scheme-less rejected", "api.github.com", "", true},
		{"no host rejected", "https://", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeAPIURL(tc.in)
			if (err != nil) != tc.wantErr {
				t.Fatalf("NormalizeAPIURL(%q) err=%v, wantErr=%v", tc.in, err, tc.wantErr)
			}
			if !tc.wantErr && got != tc.want {
				t.Fatalf("NormalizeAPIURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
