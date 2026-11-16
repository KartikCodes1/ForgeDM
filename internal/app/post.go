package app

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/KartikCodes1/forgedm/internal/engine"
	"github.com/KartikCodes1/forgedm/internal/platform"
	"github.com/KartikCodes1/forgedm/internal/postproc"
)

// finish runs the steps chosen for a completed download.
func (c *Core) finish(d engine.Download) {
	c.stats.Completed(d)
	st := c.cfg.Get()
	path := d.Path()
	var notes []string
	set := func(state string) {
		_, _ = c.eng.Update(d.ID, func(x *engine.Download) { x.PostState = state })
	}

	if d.Post.Verify || d.Checksum.Expected != "" {
		set("verifying")
		algo := d.Checksum.Algo
		if algo == "" {
			algo = "sha256"
		}
		sum, err := postproc.Hash(path, algo)
		_, _ = c.eng.Update(d.ID, func(x *engine.Download) {
			x.Checksum.Algo = algo
			x.Checksum.Actual = sum
			if err != nil {
				return
			}
			if x.Checksum.Expected != "" {
				x.Checksum.Verified = strings.EqualFold(x.Checksum.Expected, sum)
				x.Checksum.Mismatch = !x.Checksum.Verified
			}
		})
		label := strings.ToUpper(algo)
		if label == "SHA256" {
			label = "SHA-256"
		}
		switch {
		case err != nil:
			notes = append(notes, "Checksum failed: "+err.Error())
		case d.Checksum.Expected != "" && !strings.EqualFold(d.Checksum.Expected, sum):
			notes = append(notes, label+" doesn't match the expected value")
			c.notify("Checksum mismatch", d.Filename+" may be damaged or tampered with.")
		case d.Checksum.Expected != "":
			notes = append(notes, label+" verified")
		default:
			notes = append(notes, label+" calculated")
		}
	}

	if d.Post.Scan {
		set("scanning")
		res := platform.Scan(path)
		if res.Available {
			notes = append(notes, res.Note)
			if !res.Clean {
				c.notify("Threat found", d.Filename+": "+res.Note)
			}
		}
	}

	opened := path
	if d.Post.Extract && postproc.IsArchive(d.Filename) {
		set("extracting")
		dir, err := postproc.Extract(path)
		if err != nil {
			notes = append(notes, "Couldn't extract: "+err.Error())
		} else {
			notes = append(notes, "Extracted to "+dir)
			opened = dir
			if st.Settings.DeleteArchive {
				_ = os.Remove(path)
			}
		}
	}

	_, _ = c.eng.Update(d.ID, func(x *engine.Download) {
		x.PostState = ""
		if len(notes) > 0 {
			x.PostNote = strings.Join(notes, " · ")
		}
	})

	if d.Post.Open {
		_ = platform.OpenPath(opened)
	}
	c.notify("Download finished", fmt.Sprintf("%s · %s", d.Filename, humanBytes(d.Size)))
	c.pushState()
}

func (c *Core) notify(title, body string) {
	if !c.cfg.Get().Settings.Notifications {
		return
	}
	c.ui.Notify(title, body)
}

func humanBytes(n int64) string {
	if n < 0 {
		return "unknown size"
	}
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// ---- floating drop target ----

type widgetProc struct{ cmd *exec.Cmd }

// setWidget starts or stops the floating drop target window (a second
// ForgeDM process launched with --widget).
func (c *Core) setWidget(on bool) {
	c.mu.Lock()
	w := c.widget
	c.mu.Unlock()
	if on && w == nil {
		if runtime.GOOS != "windows" || c.exe == "" {
			return
		}
		cmd := exec.Command(c.exe, "--widget")
		if err := cmd.Start(); err != nil {
			c.toast("error", "The drop target couldn't open: "+err.Error())
			return
		}
		wp := &widgetProc{cmd: cmd}
		c.mu.Lock()
		c.widget = wp
		c.mu.Unlock()
		go func() {
			_ = cmd.Wait()
			c.mu.Lock()
			if c.widget == wp {
				c.widget = nil
			}
			c.mu.Unlock()
		}()
		return
	}
	if !on && w != nil {
		_ = w.cmd.Process.Kill()
		c.mu.Lock()
		c.widget = nil
		c.mu.Unlock()
	}
}
