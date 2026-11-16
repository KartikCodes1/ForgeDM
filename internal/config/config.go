// Package config holds ForgeDM's settings, save rules, queues and Site
// Grabber projects, and stores them in a JSON file.
package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/KartikCodes1/forgedm/internal/engine"
)

// Categories in display order.
var Categories = []string{"Video", "Music", "Documents", "Compressed", "Programs", "Images", "Other"}

var categoryExts = map[string][]string{
	"Video":      {"mp4", "mkv", "webm", "avi", "mov", "wmv", "flv", "m4v", "ts", "mpg", "mpeg", "3gp", "m2ts", "vob", "ogv"},
	"Music":      {"mp3", "flac", "wav", "aac", "m4a", "ogg", "opus", "wma", "m4b", "aiff", "alac", "mka"},
	"Documents":  {"pdf", "doc", "docx", "xls", "xlsx", "ppt", "pptx", "txt", "rtf", "odt", "ods", "odp", "csv", "epub", "md", "mobi", "djvu"},
	"Compressed": {"zip", "rar", "7z", "tar", "gz", "bz2", "xz", "tgz", "zst", "lz", "lzma", "cab", "tbz2", "txz"},
	"Programs":   {"exe", "msi", "msix", "msixbundle", "appx", "apk", "xapk", "dmg", "pkg", "deb", "rpm", "iso", "img", "bin", "appimage", "jar"},
	"Images":     {"jpg", "jpeg", "png", "gif", "webp", "bmp", "svg", "tif", "tiff", "heic", "heif", "avif", "ico", "psd", "raw", "cr2", "nef"},
}

var extToCategory = func() map[string]string {
	m := map[string]string{}
	for c, exts := range categoryExts {
		for _, e := range exts {
			m[e] = c
		}
	}
	return m
}()

// CategoryFor returns the category of a file name.
func CategoryFor(name string) string {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
	if c, ok := extToCategory[ext]; ok {
		return c
	}
	return "Other"
}

// CategoryExtensions exposes the built-in extension list for the UI.
func CategoryExtensions() map[string][]string { return categoryExts }

// Browser is a paired browser extension.
type Browser struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Token    string    `json:"token"`
	PairedAt time.Time `json:"pairedAt"`
	LastSeen time.Time `json:"lastSeen"`
}

// Settings are the user's preferences.
type Settings struct {
	BaseDir            string            `json:"baseDir"`
	CategoryDirs       map[string]string `json:"categoryDirs"`
	MaxActive          int               `json:"maxActive"`
	DefaultConnections int               `json:"defaultConnections"`
	SpeedLimitOn       bool              `json:"speedLimitOn"`
	SpeedLimit         int64             `json:"speedLimit"`
	ProxyMode          string            `json:"proxyMode"`
	ProxyURL           string            `json:"proxyUrl"`
	UserAgent          string            `json:"userAgent"`
	RetryCount         int               `json:"retryCount"`
	SmartPriority      bool              `json:"smartPriority"`

	CaptureEnabled bool     `json:"captureEnabled"`
	CaptureExts    []string `json:"captureExts"`
	CaptureMinSize int64    `json:"captureMinSize"`
	AskOnCapture   bool     `json:"askOnCapture"`
	ClipboardWatch bool     `json:"clipboardWatch"`
	VideoButton    bool     `json:"videoButton"`
	AltBypass      bool     `json:"altBypass"`

	VerifyChecksum  bool `json:"verifyChecksum"`
	ScanAfter       bool `json:"scanAfter"`
	OpenWhenDone    bool `json:"openWhenDone"`
	DeleteArchive   bool `json:"deleteArchive"`
	CloseToTray     bool `json:"closeToTray"`
	StartWithSystem bool `json:"startWithSystem"`
	KeepAwake       bool `json:"keepAwake"`
	Notifications   bool `json:"notifications"`
	DropTarget      bool `json:"dropTarget"`

	Theme   string `json:"theme"`
	Accent  string `json:"accent"`
	Mica    bool   `json:"mica"`
	Density string `json:"density"`

	APIPort       int       `json:"apiPort"`
	Browsers      []Browser `json:"browsers"`
	InternalToken string    `json:"internalToken"`
	Tags          []string  `json:"tags"`
	Onboarded     bool      `json:"onboarded"`
}

// Condition is one test in a save rule. All conditions must match.
// Field: ext | type | site | sizeOver | sizeUnder | name | url
type Condition struct {
	Field string `json:"field"`
	Value string `json:"value"`
}

// RuleActions say what happens when a rule matches.
type RuleActions struct {
	Dir      string   `json:"dir,omitempty"`
	Category string   `json:"category,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	Queue    string   `json:"queue,omitempty"`
	Extract  bool     `json:"extract,omitempty"`
	Open     bool     `json:"open,omitempty"`
}

// Rule is a save rule. The first enabled rule that matches wins.
type Rule struct {
	ID         string      `json:"id"`
	Name       string      `json:"name"`
	Enabled    bool        `json:"enabled"`
	Conditions []Condition `json:"conditions"`
	Actions    RuleActions `json:"actions"`
}

// Queue is a named group of downloads with an optional schedule.
type Queue struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Builtin        bool     `json:"builtin"`
	Enabled        bool     `json:"enabled"`
	Start          string   `json:"start"`
	Stop           string   `json:"stop"`
	Days           []int    `json:"days"`
	Concurrent     int      `json:"concurrent"`
	SpeedLimit     int64    `json:"speedLimit"`
	HalfBandwidth  bool     `json:"halfBandwidth"`
	WhenDone       string   `json:"whenDone"`
	WakeFromSleep  bool     `json:"wakeFromSleep"`
	RetryFailed    bool     `json:"retryFailed"`
	PauseOnMetered bool     `json:"pauseOnMetered"`
	Color          string   `json:"color"`
	Icon           string   `json:"icon"`
	Order          []string `json:"order"`
}

// Project is a saved Site Grabber scan, optionally re-run on a schedule.
type Project struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	URL           string    `json:"url"`
	Depth         int       `json:"depth"`
	SameSite      bool      `json:"sameSite"`
	Types         []string  `json:"types"`
	Exts          string    `json:"exts"`
	MinSize       int64     `json:"minSize"`
	Dir           string    `json:"dir"`
	KeepStructure bool      `json:"keepStructure"`
	SyncEnabled   bool      `json:"syncEnabled"`
	SyncDay       int       `json:"syncDay"`
	SyncTime      string    `json:"syncTime"`
	LastRun       time.Time `json:"lastRun"`
	LastFound     int       `json:"lastFound"`
}

// State is everything stored in config.json.
type State struct {
	Settings Settings  `json:"settings"`
	Rules    []Rule    `json:"rules"`
	Queues   []Queue   `json:"queues"`
	Projects []Project `json:"projects"`
}

// NewID returns a short random identifier.
func NewID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// NewToken returns a long random secret.
func NewToken() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// Defaults returns the settings for a fresh install.
func Defaults(downloadsDir string) State {
	return State{
		Settings: Settings{
			BaseDir:        filepath.Join(downloadsDir, "ForgeDM"),
			CategoryDirs:   map[string]string{},
			MaxActive:      4,
			SpeedLimit:     5 << 20,
			ProxyMode:      "system",
			RetryCount:     5,
			SmartPriority:  true,
			CaptureEnabled: true,
			CaptureExts: []string{"exe", "msi", "zip", "7z", "rar", "iso", "img", "tar", "gz", "mp4", "mkv", "webm", "avi", "mov",
				"mp3", "flac", "m4a", "wav", "pdf", "apk", "dmg", "deb", "bin", "docx", "xlsx", "pptx", "epub"},
			CaptureMinSize: 0,
			AskOnCapture:   true,
			ClipboardWatch: true,
			VideoButton:    true,
			AltBypass:      true,
			VerifyChecksum: true,
			CloseToTray:    true,
			KeepAwake:      true,
			Notifications:  true,
			Theme:          "midnight",
			Accent:         "#2EE6D6",
			Mica:           false,
			Density:        "comfortable",
			APIPort:        52771,
			InternalToken:  NewToken(),
		},
		Rules: []Rule{
			{ID: NewID(), Name: "Unpack archives", Enabled: false, Conditions: []Condition{{Field: "type", Value: "Compressed"}}, Actions: RuleActions{Extract: true}},
			{ID: NewID(), Name: "Big files wait for the night", Enabled: false, Conditions: []Condition{{Field: "sizeOver", Value: "4294967296"}}, Actions: RuleActions{Queue: "night"}},
		},
		Queues: []Queue{
			{ID: "main", Name: "Main queue", Builtin: true, Enabled: true, Concurrent: 4, WhenDone: "nothing", Color: "#2EE6D6", Icon: "bolt", RetryFailed: true},
			{ID: "night", Name: "Night queue", Enabled: true, Start: "01:00", Stop: "07:00", Days: []int{0, 1, 2, 3, 4, 5, 6}, Concurrent: 3,
				WhenDone: "nothing", WakeFromSleep: true, RetryFailed: true, PauseOnMetered: true, Color: "#A78BFA", Icon: "moon"},
		},
	}
}

// Store loads and saves the State.
type Store struct {
	mu    sync.Mutex
	path  string
	state State
}

// Open loads config.json from dir, creating defaults if needed.
func Open(dir, downloadsDir string) (*Store, error) {
	s := &Store{path: filepath.Join(dir, "config.json")}
	def := Defaults(downloadsDir)
	b, err := os.ReadFile(s.path)
	switch {
	case os.IsNotExist(err):
		s.state = def
	case err != nil:
		return nil, err
	default:
		st := def
		if err := json.Unmarshal(b, &st); err != nil {
			_ = os.Rename(s.path, s.path+".damaged")
			st = def
		}
		s.state = st
	}
	s.fill()
	return s, s.saveLocked()
}

// fill repairs missing values after upgrades or manual edits.
func (s *Store) fill() {
	st := &s.state
	if st.Settings.CategoryDirs == nil {
		st.Settings.CategoryDirs = map[string]string{}
	}
	if st.Settings.InternalToken == "" {
		st.Settings.InternalToken = NewToken()
	}
	if st.Settings.APIPort == 0 {
		st.Settings.APIPort = 52771
	}
	if st.Settings.MaxActive <= 0 {
		st.Settings.MaxActive = 4
	}
	hasMain := false
	for _, q := range st.Queues {
		if q.ID == "main" {
			hasMain = true
		}
	}
	if !hasMain {
		st.Queues = append([]Queue{Defaults("").Queues[0]}, st.Queues...)
	}
}

// Get returns a copy of the state.
func (s *Store) Get() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, _ := json.Marshal(s.state)
	var out State
	_ = json.Unmarshal(b, &out)
	return out
}

// Update changes the state and saves it.
func (s *Store) Update(fn func(st *State)) (State, error) {
	s.mu.Lock()
	fn(&s.state)
	s.fill()
	err := s.saveLocked()
	s.mu.Unlock()
	return s.Get(), err
}

func (s *Store) saveLocked() error {
	b, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}
	return engine.WriteFileAtomic(s.path, b)
}

// Path is the location of config.json.
func (s *Store) Path() string { return s.path }

// DirFor returns the save folder for a category.
func (st *Settings) DirFor(category string) string {
	if d := st.CategoryDirs[category]; d != "" {
		return d
	}
	if category == "" {
		category = "Other"
	}
	return filepath.Join(st.BaseDir, category)
}
