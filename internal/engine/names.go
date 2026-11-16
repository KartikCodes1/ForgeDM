package engine

import (
	"fmt"
	"mime"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

func joinPath(dir, name string) string { return filepath.Join(dir, name) }

var reservedNames = map[string]bool{
	"CON": true, "PRN": true, "AUX": true, "NUL": true,
	"COM1": true, "COM2": true, "COM3": true, "COM4": true, "COM5": true, "COM6": true, "COM7": true, "COM8": true, "COM9": true,
	"LPT1": true, "LPT2": true, "LPT3": true, "LPT4": true, "LPT5": true, "LPT6": true, "LPT7": true, "LPT8": true, "LPT9": true,
}

// SanitizeFilename makes a server- or user-provided name safe for Windows.
func SanitizeFilename(name string) string {
	name = strings.TrimSpace(name)
	if !utf8.ValidString(name) {
		name = strings.ToValidUTF8(name, "_")
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r < 32:
			continue
		case strings.ContainsRune(`<>:"/\|?*`, r):
			b.WriteRune('_')
		default:
			b.WriteRune(r)
		}
	}
	out := strings.Trim(b.String(), " .")
	if out == "" {
		out = "download"
	}
	base := strings.ToUpper(strings.TrimSuffix(out, filepath.Ext(out)))
	if reservedNames[base] {
		out = "_" + out
	}
	if len(out) > 200 {
		ext := filepath.Ext(out)
		if len(ext) > 20 {
			ext = ""
		}
		out = out[:200-len(ext)] + ext
	}
	return out
}

// FilenameFromDisposition extracts the filename from a Content-Disposition header.
func FilenameFromDisposition(header string) string {
	if header == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(header)
	if err == nil {
		if fn := params["filename"]; fn != "" {
			return fn
		}
	}
	// Fall back to a forgiving manual parse (many servers send malformed headers).
	lower := strings.ToLower(header)
	if i := strings.Index(lower, "filename*="); i >= 0 {
		v := header[i+len("filename*="):]
		if j := strings.Index(v, ";"); j >= 0 {
			v = v[:j]
		}
		v = strings.Trim(v, `" `)
		if k := strings.Index(v, "''"); k >= 0 {
			v = v[k+2:]
		}
		if u, err := url.PathUnescape(v); err == nil {
			return u
		}
		return v
	}
	if i := strings.Index(lower, "filename="); i >= 0 {
		v := header[i+len("filename="):]
		if j := strings.Index(v, ";"); j >= 0 {
			v = v[:j]
		}
		return strings.Trim(v, `" `)
	}
	return ""
}

// FilenameFromURL derives a name from the last path element of a URL.
func FilenameFromURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	base := path.Base(u.Path)
	if base == "/" || base == "." || base == "" {
		return ""
	}
	if s, err := url.PathUnescape(base); err == nil {
		base = s
	}
	return base
}

// ExtensionForType returns a common extension for a MIME type (with dot).
func ExtensionForType(ct string) string {
	mt, _, err := mime.ParseMediaType(ct)
	if err != nil {
		return ""
	}
	common := map[string]string{
		"application/zip": ".zip", "application/pdf": ".pdf", "video/mp4": ".mp4", "audio/mpeg": ".mp3",
		"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp", "image/gif": ".gif",
		"application/x-msdownload": ".exe", "application/x-msi": ".msi", "application/x-7z-compressed": ".7z",
		"application/vnd.rar": ".rar", "application/x-rar-compressed": ".rar", "application/gzip": ".gz",
		"video/webm": ".webm", "audio/flac": ".flac", "audio/mp4": ".m4a", "video/x-matroska": ".mkv",
		"text/plain": ".txt", "text/csv": ".csv", "application/json": ".json", "application/x-iso9660-image": ".iso",
	}
	if e, ok := common[mt]; ok {
		return e
	}
	if mt == "text/html" || mt == "application/octet-stream" {
		return ""
	}
	if exts, _ := mime.ExtensionsByType(mt); len(exts) > 0 {
		return exts[0]
	}
	return ""
}

// ResolveFilename picks the best filename from the response details.
func ResolveFilename(disposition, finalURL, origURL, contentType string) string {
	name := FilenameFromDisposition(disposition)
	if name == "" {
		name = FilenameFromURL(finalURL)
	}
	if name == "" || !strings.Contains(name, ".") {
		if n := FilenameFromURL(origURL); strings.Contains(n, ".") {
			name = n
		}
	}
	if name == "" {
		name = "download"
	}
	name = SanitizeFilename(name)
	if filepath.Ext(name) == "" {
		name += ExtensionForType(contentType)
	}
	return name
}

// UniqueName returns name, or "name (n).ext", so that it doesn't collide with
// an existing file or one of the reserved paths.
func UniqueName(dir, name string, reserved map[string]bool) string {
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	candidate := name
	for i := 1; i < 10000; i++ {
		p := strings.ToLower(filepath.Join(dir, candidate))
		_, errFinal := os.Stat(filepath.Join(dir, candidate))
		_, errTemp := os.Stat(filepath.Join(dir, candidate) + ".forgedm")
		if os.IsNotExist(errFinal) && os.IsNotExist(errTemp) && !reserved[p] {
			return candidate
		}
		candidate = fmt.Sprintf("%s (%d)%s", stem, i, ext)
	}
	return candidate
}
