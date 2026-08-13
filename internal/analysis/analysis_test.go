package analysis

import (
	"testing"

	"github.com/augusttw/procscope/internal/model"
)

func TestCompare(t *testing.T) {
	a := model.Snapshot{Process: model.Process{RSSBytes: 100, OpenFDs: 2}, Connections: []model.Connection{{Protocol: "tcp4", Local: "a", Remote: "b", State: "ESTABLISHED"}}}
	b := model.Snapshot{Process: model.Process{RSSBytes: 150, OpenFDs: 3}, Connections: []model.Connection{{Protocol: "tcp4", Local: "a", Remote: "c", State: "ESTABLISHED"}}}
	d := Compare(a, b)
	if d.Changes[1].Delta != 50 || len(d.Added) != 1 || len(d.Removed) != 1 {
		t.Fatalf("diff inesperado: %+v", d)
	}
}

func TestDoctorFindsGrowth(t *testing.T) {
	samples := []model.Snapshot{{Process: model.Process{RSSBytes: 100, OpenFDs: 10}}, {Process: model.Process{RSSBytes: 110, OpenFDs: 20}}, {Process: model.Process{RSSBytes: 140, OpenFDs: 70}}}
	f := Doctor(samples)
	codes := map[string]bool{}
	for _, v := range f {
		codes[v.Code] = true
	}
	if !codes["memory_growth"] || !codes["fd_growth"] {
		t.Fatalf("achados ausentes: %+v", f)
	}
}
