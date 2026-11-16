// Package engine implements ForgeDM's download engine: multi-connection
// segmented HTTP downloads with dynamic re-segmentation, pause/resume that
// survives restarts, retries, bandwidth limits and HLS video streams.
package engine

import (
	"sync"
	"time"
)

// Status is the lifecycle state of a download.
type Status string

const (
	StatusQueued      Status = "queued"      // waiting for its queue / a free slot
	StatusDownloading Status = "downloading" // transferring data
	StatusPaused      Status = "paused"      // stopped by the user
	StatusCompleted   Status = "completed"   // file is complete on disk
	StatusFailed      Status = "failed"      // gave up after retries
)

// Kind is the transfer protocol used for a download.
type Kind string

const (
	KindFile Kind = "file" // plain HTTP(S)/FTP-like single resource
	KindHLS  Kind = "hls"  // HTTP Live Streaming playlist (.m3u8)
)

// Error codes surfaced to the UI so it can offer the right fix.
const (
	ErrCodeLinkExpired = "link_expired"
	ErrCodeNotFound    = "not_found"
	ErrCodeDisk        = "disk"
	ErrCodeNetwork     = "network"
	ErrCodeChanged     = "changed"
	ErrCodeLive        = "live_stream"
)

// Segment is one byte range of the file. Done counts bytes written from Start.
// End is inclusive; -1 means "until the server stops sending" (unknown size).
type Segment struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
	Done  int64 `json:"done"`

	active bool
}

func (s *Segment) remaining() int64 {
	if s.End < 0 {
		return 1 << 62
	}
	return s.End - (s.Start + s.Done) + 1
}

// Checksum holds integrity information computed after a download finishes.
type Checksum struct {
	Algo     string `json:"algo,omitempty"`
	Expected string `json:"expected,omitempty"`
	Actual   string `json:"actual,omitempty"`
	Verified bool   `json:"verified,omitempty"`
	Mismatch bool   `json:"mismatch,omitempty"`
}

// PostActions are run by the app layer once the file is complete.
type PostActions struct {
	Verify  bool `json:"verify"`
	Scan    bool `json:"scan"`
	Extract bool `json:"extract"`
	Open    bool `json:"open"`
}

// HLSPart is one media segment of an HLS playlist.
type HLSPart struct {
	URL       string  `json:"url"`
	Duration  float64 `json:"dur"`
	KeyURL    string  `json:"key,omitempty"`
	KeyIV     string  `json:"iv,omitempty"`
	Seq       int64   `json:"seq"`
	Size      int64   `json:"size"`
	Done      bool    `json:"done"`
	ByteRange string  `json:"range,omitempty"`
}

// HLSState tracks an HLS download.
type HLSState struct {
	PlaylistURL string    `json:"playlist"`
	Parts       []HLSPart `json:"parts"`
	Duration    float64   `json:"duration"`
}

// Download is the persisted record of one transfer.
type Download struct {
	ID          string            `json:"id"`
	URL         string            `json:"url"`
	FinalURL    string            `json:"finalUrl,omitempty"`
	PageURL     string            `json:"pageUrl,omitempty"`
	Kind        Kind              `json:"kind"`
	Filename    string            `json:"filename"`
	NameFixed   bool              `json:"nameFixed,omitempty"`
	Dir         string            `json:"dir"`
	Size        int64             `json:"size"`
	Downloaded  int64             `json:"downloaded"`
	Resumable   bool              `json:"resumable"`
	Status      Status            `json:"status"`
	Error       string            `json:"error,omitempty"`
	ErrorCode   string            `json:"errorCode,omitempty"`
	Category    string            `json:"category"`
	Tags        []string          `json:"tags,omitempty"`
	Queue       string            `json:"queue"`
	Connections int               `json:"connections"`
	SpeedLimit  int64             `json:"speedLimit,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Segments    []*Segment        `json:"segments,omitempty"`
	ETag        string            `json:"etag,omitempty"`
	LastMod     string            `json:"lastModified,omitempty"`
	ContentType string            `json:"contentType,omitempty"`
	Checksum    Checksum          `json:"checksum"`
	Post        PostActions       `json:"post"`
	PostState   string            `json:"postState,omitempty"`
	PostNote    string            `json:"postNote,omitempty"`
	Source      string            `json:"source,omitempty"`
	Rule        string            `json:"rule,omitempty"`
	HLS         *HLSState         `json:"hls,omitempty"`
	CreatedAt   time.Time         `json:"createdAt"`
	StartedAt   time.Time         `json:"startedAt,omitempty"`
	CompletedAt time.Time         `json:"completedAt,omitempty"`
	ActiveMs    int64             `json:"activeMs"`
	SavedMs     int64             `json:"savedMs"`
	Manual      bool              `json:"manual,omitempty"`
}

// Path is the final location of the file.
func (d *Download) Path() string { return joinPath(d.Dir, d.Filename) }

// TempPath is where data is written while downloading.
func (d *Download) TempPath() string { return d.Path() + ".forgedm" }

// Live is the runtime view of a download, sent to the UI several times a second.
type Live struct {
	ID         string     `json:"id"`
	Status     Status     `json:"status"`
	Downloaded int64      `json:"downloaded"`
	Size       int64      `json:"size"`
	Speed      float64    `json:"speed"`
	ETA        float64    `json:"eta"`
	Conns      int        `json:"conns"`
	Map        []float64  `json:"map"`
	PeakSpeed  float64    `json:"peak"`
	History    []float64  `json:"history,omitempty"`
	Parts      []PartView `json:"parts,omitempty"`
}

// PartView describes one piece of the file for the progress window.
type PartView struct {
	Start  int64 `json:"start"`
	End    int64 `json:"end"`
	Done   int64 `json:"done"`
	Active bool  `json:"active"`
}

// Snapshot returns a deep-enough copy safe to hand to other goroutines.
func (d *Download) Snapshot() Download {
	c := *d
	if d.Headers != nil {
		c.Headers = make(map[string]string, len(d.Headers))
		for k, v := range d.Headers {
			c.Headers[k] = v
		}
	}
	c.Tags = append([]string(nil), d.Tags...)
	c.Segments = make([]*Segment, len(d.Segments))
	for i, s := range d.Segments {
		cs := *s
		c.Segments[i] = &cs
	}
	if d.HLS != nil {
		h := *d.HLS
		h.Parts = append([]HLSPart(nil), d.HLS.Parts...)
		c.HLS = &h
	}
	return c
}

// Request describes a new download.
type Request struct {
	URL         string            `json:"url"`
	PageURL     string            `json:"pageUrl,omitempty"`
	Kind        Kind              `json:"kind,omitempty"`
	Filename    string            `json:"filename,omitempty"`
	Dir         string            `json:"dir"`
	Headers     map[string]string `json:"headers,omitempty"`
	Category    string            `json:"category,omitempty"`
	Tags        []string          `json:"tags,omitempty"`
	Queue       string            `json:"queue,omitempty"`
	Connections int               `json:"connections,omitempty"`
	SpeedLimit  int64             `json:"speedLimit,omitempty"`
	Post        PostActions       `json:"post"`
	Checksum    string            `json:"checksum,omitempty"`
	Start       bool              `json:"start"`
	Source      string            `json:"source,omitempty"`
	Rule        string            `json:"rule,omitempty"`
	Size        int64             `json:"size,omitempty"`
}

// ProbeResult is what the server tells us about a URL before downloading.
type ProbeResult struct {
	URL         string       `json:"url"`
	FinalURL    string       `json:"finalUrl"`
	Filename    string       `json:"filename"`
	Size        int64        `json:"size"`
	Resumable   bool         `json:"resumable"`
	ContentType string       `json:"contentType"`
	Kind        Kind         `json:"kind"`
	ETag        string       `json:"etag,omitempty"`
	LastMod     string       `json:"lastModified,omitempty"`
	Variants    []HLSVariant `json:"variants,omitempty"`
}

// HLSVariant is one quality level from a master playlist.
type HLSVariant struct {
	URL        string `json:"url"`
	Bandwidth  int64  `json:"bandwidth"`
	Resolution string `json:"resolution"`
	Label      string `json:"label"`
}

// task is the runtime state of a download held by the engine.
type task struct {
	mu   sync.Mutex
	d    *Download
	run  *runState
	peak float64
}
