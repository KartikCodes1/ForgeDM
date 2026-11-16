package rules

import (
	"path/filepath"
	"testing"

	"github.com/KartikCodes1/forgedm/internal/config"
)

func TestDecide(t *testing.T) {
	st := config.Defaults("/home/u/Downloads")
	st.Rules = []config.Rule{
		{ID: "1", Name: "ISOs", Enabled: true, Conditions: []config.Condition{{Field: "ext", Value: ".iso, .img"}}, Actions: config.RuleActions{Dir: "/d/ISOs", Tags: []string{"iso"}}},
		{ID: "2", Name: "Off", Enabled: false, Conditions: []config.Condition{{Field: "type", Value: "Video"}}, Actions: config.RuleActions{Dir: "/never"}},
		{ID: "3", Name: "Example programs", Enabled: true, Conditions: []config.Condition{{Field: "site", Value: "example.com"}, {Field: "type", Value: "Programs"}}, Actions: config.RuleActions{Dir: "/d/Installers"}},
		{ID: "4", Name: "Big", Enabled: true, Conditions: []config.Condition{{Field: "sizeOver", Value: "1000"}}, Actions: config.RuleActions{Queue: "night"}},
	}
	cases := []struct {
		in       Input
		dir      string
		queue    string
		category string
	}{
		{Input{URL: "https://x.org/ubuntu.iso", Filename: "ubuntu.iso", Size: 10}, "/d/ISOs", "main", "Programs"},
		{Input{URL: "https://downloads.example.com/a/setup.exe", Filename: "setup.exe", Size: 10}, "/d/Installers", "main", "Programs"},
		{Input{URL: "https://other.org/setup.exe", Filename: "setup.exe", Size: 10}, filepath.Join("/home/u/Downloads/ForgeDM", "Programs"), "main", "Programs"},
		{Input{URL: "https://other.org/movie.mkv", Filename: "movie.mkv", Size: 5000}, filepath.Join("/home/u/Downloads/ForgeDM", "Video"), "night", "Video"},
		{Input{URL: "https://other.org/movie.mkv", Filename: "movie.mkv", Size: -1}, filepath.Join("/home/u/Downloads/ForgeDM", "Video"), "main", "Video"},
	}
	for _, c := range cases {
		d := Decide(st, c.in)
		if d.Dir != c.dir || d.Queue != c.queue || d.Category != c.category {
			t.Errorf("%s: got dir=%s queue=%s cat=%s", c.in.Filename, d.Dir, d.Queue, d.Category)
		}
	}
}
