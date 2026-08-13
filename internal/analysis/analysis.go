package analysis

import (
	"fmt"
	"math"

	"github.com/augusttw/procscope/internal/model"
)

type Change struct {
	Metric string  `json:"metric"`
	Before float64 `json:"before"`
	After  float64 `json:"after"`
	Delta  float64 `json:"delta"`
}
type Diff struct {
	Changes []Change           `json:"changes"`
	Added   []model.Connection `json:"added_connections,omitempty"`
	Removed []model.Connection `json:"removed_connections,omitempty"`
}

func Compare(a, b model.Snapshot) Diff {
	d := Diff{Changes: []Change{
		change("cpu_percent", a.Process.CPUPercent, b.Process.CPUPercent),
		change("rss_bytes", float64(a.Process.RSSBytes), float64(b.Process.RSSBytes)),
		change("vm_bytes", float64(a.Process.VMSBytes), float64(b.Process.VMSBytes)),
		change("open_fds", float64(a.Process.OpenFDs), float64(b.Process.OpenFDs)),
		change("threads", float64(a.Process.Threads), float64(b.Process.Threads)),
	}}
	ac, bc := connectionSet(a.Connections), connectionSet(b.Connections)
	for k, c := range bc {
		if _, ok := ac[k]; !ok {
			d.Added = append(d.Added, c)
		}
	}
	for k, c := range ac {
		if _, ok := bc[k]; !ok {
			d.Removed = append(d.Removed, c)
		}
	}
	return d
}
func change(m string, a, b float64) Change {
	return Change{Metric: m, Before: a, After: b, Delta: b - a}
}
func connectionSet(cs []model.Connection) map[string]model.Connection {
	m := map[string]model.Connection{}
	for _, c := range cs {
		m[c.Protocol+c.Local+c.Remote+c.State] = c
	}
	return m
}

type Severity string

const (
	Info     Severity = "info"
	Warning  Severity = "warning"
	Critical Severity = "critical"
)

type Finding struct {
	Severity   Severity `json:"severity"`
	Code       string   `json:"code"`
	Message    string   `json:"message"`
	Suggestion string   `json:"suggestion,omitempty"`
}

func Doctor(samples []model.Snapshot) []Finding {
	if len(samples) == 0 {
		return nil
	}
	last := samples[len(samples)-1]
	p := last.Process
	var out []Finding
	if p.CPUPercent >= 90 {
		out = append(out, Finding{Critical, "high_cpu", fmt.Sprintf("CPU em %.1f%%", p.CPUPercent), "Investigue loops quentes com pprof ou perf."})
	} else if p.CPUPercent >= 70 {
		out = append(out, Finding{Warning, "elevated_cpu", fmt.Sprintf("CPU em %.1f%%", p.CPUPercent), "Observe por mais tempo para confirmar saturação."})
	}
	if p.OpenFDs >= 1024 {
		out = append(out, Finding{Critical, "many_fds", fmt.Sprintf("%d descritores abertos", p.OpenFDs), "Verifique vazamentos e o limite RLIMIT_NOFILE."})
	} else if p.OpenFDs >= 512 {
		out = append(out, Finding{Warning, "many_fds", fmt.Sprintf("%d descritores abertos", p.OpenFDs), "Inspecione sockets e arquivos não fechados."})
	}
	if p.State == "zombie" || p.State == "disk sleep" {
		out = append(out, Finding{Critical, "bad_state", "Processo em estado " + p.State, "Verifique o processo pai ou bloqueios de I/O."})
	}
	if len(samples) >= 3 {
		first := samples[0].Process
		if first.RSSBytes > 0 {
			growth := float64(p.RSSBytes-first.RSSBytes) / float64(first.RSSBytes)
			if growth >= .25 {
				out = append(out, Finding{Warning, "memory_growth", fmt.Sprintf("RSS cresceu %.0f%% durante o trace", growth*100), "Amplie a gravação e procure retenção de memória."})
			}
		}
		if p.OpenFDs-first.OpenFDs >= 50 {
			out = append(out, Finding{Warning, "fd_growth", fmt.Sprintf("descritores cresceram de %d para %d", first.OpenFDs, p.OpenFDs), "Confirme se recursos são fechados após o uso."})
		}
	}
	timeWait := 0
	for _, c := range last.Connections {
		if c.State == "TIME_WAIT" {
			timeWait++
		}
	}
	if timeWait >= 100 {
		out = append(out, Finding{Warning, "many_time_wait", fmt.Sprintf("%d conexões em TIME_WAIT", timeWait), "Reutilize conexões e revise pools HTTP/DB."})
	}
	if len(out) == 0 {
		out = append(out, Finding{Info, "healthy", "Nenhum comportamento suspeito detectado", "Grave um trace mais longo para uma análise mais representativa."})
	}
	return out
}

func Significant(c Change) bool { return math.Abs(c.Delta) > .0001 }
