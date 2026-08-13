package format

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/augusttw/procscope/internal/model"
)

func Bytes(v uint64) string {
	const u = 1024
	if v < u {
		return fmt.Sprintf("%d B", v)
	}
	div := uint64(u)
	exp := 0
	for n := v / u; n >= u; n /= u {
		div *= u
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(v)/float64(div), "KMGTPE"[exp])
}
func Duration(d time.Duration) string {
	if d < time.Second {
		return d.Round(time.Millisecond).String()
	}
	return d.Round(time.Second).String()
}
func Snapshot(w io.Writer, s model.Snapshot) {
	p := s.Process
	fmt.Fprintf(w, "PID %-7d %-20s %s\n", p.PID, p.Name, p.State)
	fmt.Fprintf(w, "CPU %-7.1f%% RSS %-10s VM %-10s uptime %s\n", p.CPUPercent, Bytes(p.RSSBytes), Bytes(p.VMSBytes), Duration(p.Uptime))
	fmt.Fprintf(w, "threads %-5d FDs %-7d I/O read %s write %s\n", p.Threads, p.OpenFDs, Bytes(p.ReadBytes), Bytes(p.WriteBytes))
	if len(s.Connections) > 0 {
		fmt.Fprintln(w, "\nPROTO  STATE         LOCAL                    REMOTE                   DEPENDENCY")
		for _, c := range s.Connections {
			fmt.Fprintf(w, "%-6s %-13s %-24s %-24s %s\n", c.Protocol, c.State, c.Local, c.Remote, c.Dependency)
		}
	}
}
func Event(w io.Writer, s model.Snapshot) {
	deps := map[string]bool{}
	for _, c := range s.Connections {
		if c.Dependency != "" {
			deps[c.Dependency] = true
		}
	}
	var d []string
	for v := range deps {
		d = append(d, v)
	}
	fmt.Fprintf(w, "%s  cpu %6.1f%%  rss %-9s  fds %-5d  tcp %-3d", s.CapturedAt.Format("15:04:05"), s.Process.CPUPercent, Bytes(s.Process.RSSBytes), s.Process.OpenFDs, len(s.Connections))
	if len(d) > 0 {
		fmt.Fprintf(w, "  deps %s", strings.Join(d, ","))
	}
	fmt.Fprintln(w)
}
