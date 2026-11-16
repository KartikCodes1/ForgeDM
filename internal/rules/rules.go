// Package rules decides where a new download goes: its category, folder,
// queue, tags and finishing actions, based on the user's save rules.
package rules

import (
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/KartikCodes1/forgedm/internal/config"
)

// Input describes a download about to be added.
type Input struct {
	URL      string
	PageURL  string
	Filename string
	Size     int64
}

// Decision is the outcome of applying the rules.
type Decision struct {
	Category string   `json:"category"`
	Dir      string   `json:"dir"`
	Tags     []string `json:"tags"`
	Queue    string   `json:"queue"`
	Extract  bool     `json:"extract"`
	Open     bool     `json:"open"`
	RuleID   string   `json:"ruleId"`
	RuleName string   `json:"ruleName"`
}

// Decide applies the first matching enabled rule.
func Decide(st config.State, in Input) Decision {
	cat := config.CategoryFor(in.Filename)
	d := Decision{Category: cat, Queue: "main"}
	for _, r := range st.Rules {
		if !r.Enabled || len(r.Conditions) == 0 || !Matches(r, in, cat) {
			continue
		}
		a := r.Actions
		if a.Category != "" {
			d.Category = a.Category
		}
		d.Dir = a.Dir
		d.Tags = append([]string(nil), a.Tags...)
		if a.Queue != "" {
			d.Queue = a.Queue
		}
		d.Extract = a.Extract
		d.Open = a.Open
		d.RuleID, d.RuleName = r.ID, r.Name
		break
	}
	if d.Dir == "" {
		d.Dir = st.Settings.DirFor(d.Category)
	}
	return d
}

// Matches reports whether every condition of r holds for in.
func Matches(r config.Rule, in Input, category string) bool {
	for _, c := range r.Conditions {
		if !matchOne(c, in, category) {
			return false
		}
	}
	return true
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
}

func siteMatches(host, want string) bool {
	if host == "" {
		return false
	}
	return host == want || strings.HasSuffix(host, "."+want)
}

// ParseList splits ".iso, .img" or "iso img" into {"iso","img"}.
func ParseList(v string) []string {
	f := strings.FieldsFunc(strings.ToLower(v), func(r rune) bool { return r == ',' || r == ' ' || r == ';' })
	out := make([]string, 0, len(f))
	for _, x := range f {
		x = strings.TrimPrefix(strings.TrimSpace(x), "*")
		x = strings.TrimPrefix(x, ".")
		if x != "" {
			out = append(out, x)
		}
	}
	return out
}

func matchOne(c config.Condition, in Input, category string) bool {
	v := strings.TrimSpace(c.Value)
	switch c.Field {
	case "ext":
		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(in.Filename)), ".")
		for _, e := range ParseList(v) {
			if e == ext {
				return true
			}
		}
		return false
	case "type":
		return strings.EqualFold(category, v)
	case "site":
		want := strings.TrimPrefix(strings.TrimPrefix(strings.ToLower(v), "*."), "www.")
		want = strings.TrimSuffix(strings.TrimPrefix(strings.TrimPrefix(want, "https://"), "http://"), "/")
		return siteMatches(hostOf(in.URL), want) || siteMatches(hostOf(in.PageURL), want)
	case "sizeOver":
		n, err := strconv.ParseInt(v, 10, 64)
		return err == nil && in.Size > 0 && in.Size > n
	case "sizeUnder":
		n, err := strconv.ParseInt(v, 10, 64)
		return err == nil && in.Size >= 0 && in.Size < n
	case "name":
		name := strings.ToLower(in.Filename)
		pat := strings.ToLower(v)
		if strings.ContainsAny(pat, "*?") {
			ok, _ := filepath.Match(pat, name)
			return ok
		}
		return strings.Contains(name, pat)
	case "url":
		return strings.Contains(strings.ToLower(in.URL), strings.ToLower(v))
	}
	return false
}
