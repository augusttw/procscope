package storage

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/augusttw/procscope/internal/model"
)

func TestTraceRoundTripAndSnapshotProjection(t *testing.T) {
	p := filepath.Join(t.TempDir(), "trace.json")
	trace := model.Trace{Schema: 1, StartedAt: time.Now(), Samples: []model.Snapshot{{Schema: 1, Process: model.Process{PID: 7}}, {Schema: 1, Process: model.Process{PID: 8}}}}
	if err := Save(p, trace); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSnapshot(p)
	if err != nil {
		t.Fatal(err)
	}
	if got.Process.PID != 8 {
		t.Fatalf("PID=%d", got.Process.PID)
	}
}
