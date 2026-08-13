package observe

import (
	"context"
	"time"

	"github.com/augusttw/procscope/internal/model"
)

// Source is intentionally independent of /proc so Netlink, eBPF and remote
// collectors can be introduced without changing commands or analysis.
type Source interface {
	Snapshot(context.Context, int) (model.Snapshot, error)
}

type Sampler struct {
	Source   Source
	Interval time.Duration
}

func (s Sampler) Stream(ctx context.Context, pid int) (<-chan model.Snapshot, <-chan error) {
	out := make(chan model.Snapshot)
	errs := make(chan error, 1)
	go func() {
		defer close(out)
		defer close(errs)
		interval := s.Interval
		if interval <= 0 {
			interval = time.Second
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			snap, err := s.Source.Snapshot(ctx, pid)
			if err != nil {
				errs <- err
				return
			}
			select {
			case out <- snap:
			case <-ctx.Done():
				return
			}
			select {
			case <-ticker.C:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, errs
}
