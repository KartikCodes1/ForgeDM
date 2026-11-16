package engine

import (
	"bufio"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type playlist struct {
	master    bool
	variants  []HLSVariant
	parts     []HLSPart
	endList   bool
	sampleAES bool
	duration  float64
	fmp4      bool
}

// parseAttrs splits `A=1,B="x,y",C=z` honoring quotes.
func parseAttrs(s string) map[string]string {
	out := map[string]string{}
	var key strings.Builder
	var val strings.Builder
	inKey, inQuote := true, false
	flush := func() {
		k := strings.TrimSpace(key.String())
		if k != "" {
			out[strings.ToUpper(k)] = strings.Trim(strings.TrimSpace(val.String()), `"`)
		}
		key.Reset()
		val.Reset()
		inKey = true
	}
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
			val.WriteRune(r)
		case r == ',' && !inQuote:
			flush()
		case r == '=' && inKey && !inQuote:
			inKey = false
		case inKey:
			key.WriteRune(r)
		default:
			val.WriteRune(r)
		}
	}
	flush()
	return out
}

func resolveRef(base *url.URL, ref string) string {
	u, err := url.Parse(strings.TrimSpace(ref))
	if err != nil {
		return ref
	}
	return base.ResolveReference(u).String()
}

func parsePlaylist(base *url.URL, body string) (*playlist, error) {
	sc := bufio.NewScanner(strings.NewReader(body))
	sc.Buffer(make([]byte, 1<<20), 8<<20)
	pl := &playlist{}
	first := true
	var pendingInf *HLSVariant
	var dur float64
	haveDur := false
	var keyURL, keyIV string
	var seq int64
	var byteRange string
	var lastRangeEnd = map[string]int64{}
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if first {
			first = false
			if !strings.HasPrefix(line, "#EXTM3U") {
				return nil, errors.New("this link isn't a video playlist")
			}
			continue
		}
		switch {
		case strings.HasPrefix(line, "#EXT-X-STREAM-INF:"):
			a := parseAttrs(strings.TrimPrefix(line, "#EXT-X-STREAM-INF:"))
			bw, _ := strconv.ParseInt(a["BANDWIDTH"], 10, 64)
			pendingInf = &HLSVariant{Bandwidth: bw, Resolution: a["RESOLUTION"]}
			pl.master = true
		case strings.HasPrefix(line, "#EXTINF:"):
			v := strings.TrimPrefix(line, "#EXTINF:")
			if i := strings.Index(v, ","); i >= 0 {
				v = v[:i]
			}
			dur, _ = strconv.ParseFloat(strings.TrimSpace(v), 64)
			haveDur = true
		case strings.HasPrefix(line, "#EXT-X-MEDIA-SEQUENCE:"):
			seq, _ = strconv.ParseInt(strings.TrimPrefix(line, "#EXT-X-MEDIA-SEQUENCE:"), 10, 64)
		case strings.HasPrefix(line, "#EXT-X-KEY:"):
			a := parseAttrs(strings.TrimPrefix(line, "#EXT-X-KEY:"))
			switch strings.ToUpper(a["METHOD"]) {
			case "NONE", "":
				keyURL, keyIV = "", ""
			case "AES-128":
				keyURL = resolveRef(base, a["URI"])
				keyIV = a["IV"]
			default:
				pl.sampleAES = true
			}
		case strings.HasPrefix(line, "#EXT-X-MAP:"):
			a := parseAttrs(strings.TrimPrefix(line, "#EXT-X-MAP:"))
			if a["URI"] != "" {
				pl.fmp4 = true
				part := HLSPart{URL: resolveRef(base, a["URI"]), Seq: -1}
				if br := a["BYTERANGE"]; br != "" {
					part.ByteRange = normalizeRange(br, part.URL, lastRangeEnd)
				}
				pl.parts = append(pl.parts, part)
			}
		case strings.HasPrefix(line, "#EXT-X-BYTERANGE:"):
			byteRange = strings.TrimPrefix(line, "#EXT-X-BYTERANGE:")
		case strings.HasPrefix(line, "#EXT-X-ENDLIST"):
			pl.endList = true
		case strings.HasPrefix(line, "#EXT-X-PLAYLIST-TYPE:VOD"):
			pl.endList = true
		case strings.HasPrefix(line, "#"):
			// other tags are not needed for downloading
		default:
			ref := resolveRef(base, line)
			if pendingInf != nil {
				pendingInf.URL = ref
				pl.variants = append(pl.variants, *pendingInf)
				pendingInf = nil
				continue
			}
			if haveDur {
				p := HLSPart{URL: ref, Duration: dur, KeyURL: keyURL, KeyIV: keyIV, Seq: seq}
				if byteRange != "" {
					p.ByteRange = normalizeRange(byteRange, ref, lastRangeEnd)
					byteRange = ""
				}
				pl.parts = append(pl.parts, p)
				pl.duration += dur
				seq++
				haveDur = false
			}
		}
	}
	for i := range pl.variants {
		pl.variants[i].Label = variantLabel(pl.variants[i])
	}
	sort.SliceStable(pl.variants, func(i, j int) bool { return pl.variants[i].Bandwidth > pl.variants[j].Bandwidth })
	return pl, sc.Err()
}

// normalizeRange turns "len[@off]" into "start-end".
func normalizeRange(br, ref string, last map[string]int64) string {
	var length, off int64
	if i := strings.Index(br, "@"); i >= 0 {
		length, _ = strconv.ParseInt(br[:i], 10, 64)
		off, _ = strconv.ParseInt(br[i+1:], 10, 64)
	} else {
		length, _ = strconv.ParseInt(br, 10, 64)
		off = last[ref]
	}
	last[ref] = off + length
	return fmt.Sprintf("%d-%d", off, off+length-1)
}

func variantLabel(v HLSVariant) string {
	if v.Resolution != "" {
		if i := strings.Index(strings.ToLower(v.Resolution), "x"); i >= 0 {
			return v.Resolution[i+1:] + "p"
		}
		return v.Resolution
	}
	if v.Bandwidth > 0 {
		return fmt.Sprintf("%d kbps", v.Bandwidth/1000)
	}
	return "Video"
}

func (e *Engine) fetchText(ctx context.Context, d *Download, u string) (string, *url.URL, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", nil, &fatalError{err}
	}
	e.applyHeaders(req, d.Headers)
	e.mu.Lock()
	client := e.client
	e.mu.Unlock()
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", nil, &httpError{Code: resp.StatusCode, Status: resp.Status}
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return "", nil, err
	}
	return string(b), resp.Request.URL, nil
}

// loadMedia resolves a (possibly master) playlist to a media playlist.
func (e *Engine) loadMedia(ctx context.Context, d *Download, u string) (*playlist, []HLSVariant, string, error) {
	text, base, err := e.fetchText(ctx, d, u)
	if err != nil {
		return nil, nil, "", err
	}
	pl, err := parsePlaylist(base, text)
	if err != nil {
		return nil, nil, "", &fatalError{err}
	}
	if pl.master {
		if len(pl.variants) == 0 {
			return nil, nil, "", &fatalError{errors.New("the playlist has no video streams")}
		}
		variants := pl.variants
		mediaURL := variants[0].URL
		text, base, err = e.fetchText(ctx, d, mediaURL)
		if err != nil {
			return nil, nil, "", err
		}
		pl, err = parsePlaylist(base, text)
		if err != nil {
			return nil, nil, "", &fatalError{err}
		}
		return pl, variants, mediaURL, nil
	}
	return pl, nil, u, nil
}

func (e *Engine) probeHLS(ctx context.Context, d *Download) (ProbeResult, error) {
	res := ProbeResult{URL: d.URL, FinalURL: d.URL, Kind: KindHLS, Size: -1, Resumable: true, ContentType: "application/vnd.apple.mpegurl"}
	text, base, err := e.fetchText(ctx, d, d.URL)
	if err != nil {
		return res, err
	}
	pl, err := parsePlaylist(base, text)
	if err != nil {
		return res, err
	}
	res.FinalURL = base.String()
	res.Variants = pl.variants
	stem := strings.TrimSuffix(SanitizeFilename(FilenameFromURL(d.URL)), filepath.Ext(FilenameFromURL(d.URL)))
	if stem == "" || stem == "download" || strings.EqualFold(stem, "index") || strings.EqualFold(stem, "master") || strings.EqualFold(stem, "playlist") {
		stem = "video"
	}
	ext := ".ts"
	if !pl.master && pl.fmp4 {
		ext = ".mp4"
	}
	res.Filename = stem + ext
	return res, nil
}

func (e *Engine) runHLS(t *task, rs *runState) error {
	ctx := rs.ctx
	t.mu.Lock()
	d := t.d
	needPlan := d.HLS == nil || len(d.HLS.Parts) == 0
	t.mu.Unlock()

	if needPlan {
		pl, _, mediaURL, err := e.loadMedia(ctx, d, d.URL)
		if err != nil {
			return err
		}
		if pl.sampleAES {
			return &codedError{code: ErrCodeLive, msg: "This video is copy-protected, so it can't be downloaded."}
		}
		if !pl.endList {
			return &codedError{code: ErrCodeLive, msg: "This is a live stream. ForgeDM can only download videos that have an end."}
		}
		if len(pl.parts) == 0 {
			return &fatalError{errors.New("the playlist is empty")}
		}
		t.mu.Lock()
		d.HLS = &HLSState{PlaylistURL: mediaURL, Parts: pl.parts, Duration: pl.duration}
		d.Resumable = true
		ext := ".ts"
		if pl.fmp4 {
			ext = ".mp4"
		}
		cur := strings.ToLower(filepath.Ext(d.Filename))
		if cur == ".m3u8" || cur == "" || (cur != ext && (cur == ".ts" || cur == ".mp4")) {
			name := strings.TrimSuffix(d.Filename, filepath.Ext(d.Filename))
			if name == "" {
				name = "video"
			}
			d.Filename = SanitizeFilename(name + ext)
		}
		d.Downloaded = 0
		d.Size = -1
		t.mu.Unlock()
		e.markDirty()
		e.lis.Changed(t.snapshot())
	}

	dir := tempDirFor(d)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return &fatalError{err}
	}

	e.mu.Lock()
	cfg := e.cfg
	e.mu.Unlock()
	conns := d.Connections
	if conns <= 0 {
		conns = 8
	}
	if conns > len(rs.wbytes) {
		conns = len(rs.wbytes)
	}

	jobs := make(chan int)
	var wg sync.WaitGroup
	keys := &keyCache{m: map[string][]byte{}}
	for w := 0; w < conns; w++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			rs.conns.Add(1)
			defer rs.conns.Add(-1)
			for idx := range jobs {
				var err error
				for attempt := 0; attempt <= cfg.RetryCount; attempt++ {
					err = e.fetchPart(t, rs, id, idx, dir, keys)
					if err == nil || rs.ctx.Err() != nil || isFatal(err) {
						break
					}
					select {
					case <-rs.ctx.Done():
					case <-time.After(cfg.RetryDelay << attempt):
					}
				}
				if err != nil && rs.ctx.Err() == nil {
					rs.fail(err)
				}
			}
		}(w)
	}
	t.mu.Lock()
	n := len(d.HLS.Parts)
	var todo []int
	for i, p := range d.HLS.Parts {
		if !p.Done {
			todo = append(todo, i)
		}
	}
	t.mu.Unlock()
feed:
	for _, i := range todo {
		select {
		case <-ctx.Done():
			break feed
		case jobs <- i:
		}
	}
	close(jobs)
	wg.Wait()
	if ctx.Err() != nil {
		if rs.failErr != nil {
			return rs.failErr
		}
		return context.Canceled
	}

	// Join the pieces in order.
	out, err := os.Create(d.TempPath())
	if err != nil {
		return &fatalError{err}
	}
	var total int64
	for i := 0; i < n; i++ {
		in, err := os.Open(filepath.Join(dir, fmt.Sprintf("%06d", i)))
		if err != nil {
			out.Close()
			return &fatalError{err}
		}
		c, err := io.Copy(out, in)
		in.Close()
		if err != nil {
			out.Close()
			return &fatalError{err}
		}
		total += c
	}
	if err := out.Close(); err != nil {
		return &fatalError{err}
	}
	_ = os.RemoveAll(dir)
	t.mu.Lock()
	d.Size = total
	t.mu.Unlock()
	return e.finalize(t)
}

type keyCache struct {
	mu sync.Mutex
	m  map[string][]byte
}

func (e *Engine) key(ctx context.Context, d *Download, kc *keyCache, u string) ([]byte, error) {
	kc.mu.Lock()
	k, ok := kc.m[u]
	kc.mu.Unlock()
	if ok {
		return k, nil
	}
	text, _, err := e.fetchText(ctx, d, u)
	if err != nil {
		return nil, err
	}
	k = []byte(text)
	if len(k) != 16 {
		return nil, &fatalError{errors.New("unexpected video key format")}
	}
	kc.mu.Lock()
	kc.m[u] = k
	kc.mu.Unlock()
	return k, nil
}

func (e *Engine) fetchPart(t *task, rs *runState, id, idx int, dir string, kc *keyCache) error {
	t.mu.Lock()
	d := t.d
	p := d.HLS.Parts[idx]
	t.mu.Unlock()
	req, err := http.NewRequestWithContext(rs.ctx, http.MethodGet, p.URL, nil)
	if err != nil {
		return &fatalError{err}
	}
	e.applyHeaders(req, d.Headers)
	if p.ByteRange != "" {
		req.Header.Set("Range", "bytes="+p.ByteRange)
	}
	e.mu.Lock()
	client := e.client
	e.mu.Unlock()
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return &httpError{Code: resp.StatusCode, Status: resp.Status}
	}
	var data []byte
	buf := make([]byte, readChunk)
	for {
		if err := e.wait(rs, len(buf)); err != nil {
			return err
		}
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			data = append(data, buf[:n]...)
			rs.bytes.Add(int64(n))
			rs.wbytes[id].Add(int64(n))
			e.totalBytes.Add(int64(n))
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return rerr
		}
	}
	if p.KeyURL != "" {
		key, err := e.key(rs.ctx, d, kc, p.KeyURL)
		if err != nil {
			return err
		}
		iv := make([]byte, 16)
		if p.KeyIV != "" {
			h := strings.TrimPrefix(strings.TrimPrefix(p.KeyIV, "0x"), "0X")
			b, err := hex.DecodeString(fmt.Sprintf("%032s", h))
			if err != nil || len(b) != 16 {
				return &fatalError{errors.New("bad video key IV")}
			}
			iv = b
		} else {
			binary.BigEndian.PutUint64(iv[8:], uint64(p.Seq))
		}
		if len(data)%aes.BlockSize != 0 {
			return errors.New("video piece has the wrong length")
		}
		block, err := aes.NewCipher(key)
		if err != nil {
			return &fatalError{err}
		}
		cipher.NewCBCDecrypter(block, iv).CryptBlocks(data, data)
		if l := len(data); l > 0 {
			pad := int(data[l-1])
			if pad > 0 && pad <= aes.BlockSize && pad <= l {
				data = data[:l-pad]
			}
		}
	}
	if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("%06d", idx)), data, 0o644); err != nil {
		return &fatalError{err}
	}
	t.mu.Lock()
	d.HLS.Parts[idx].Done = true
	d.HLS.Parts[idx].Size = int64(len(data))
	d.Downloaded += int64(len(data))
	done, sum := 0, int64(0)
	for _, x := range d.HLS.Parts {
		if x.Done {
			done++
			sum += x.Size
		}
	}
	if done > 0 {
		d.Size = sum * int64(len(d.HLS.Parts)) / int64(done)
	}
	t.mu.Unlock()
	return nil
}
