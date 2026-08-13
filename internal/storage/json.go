package storage

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/augusttw/procscope/internal/model"
)

func Save(path string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return fmt.Errorf("salvar %s: %w", path, err)
	}
	return nil
}
func LoadSnapshot(path string) (model.Snapshot, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return model.Snapshot{}, err
	}
	var s model.Snapshot
	if err = json.Unmarshal(b, &s); err == nil && s.Process.PID != 0 {
		return s, nil
	}
	var t model.Trace
	if err = json.Unmarshal(b, &t); err != nil {
		return model.Snapshot{}, fmt.Errorf("JSON inválido: %w", err)
	}
	if len(t.Samples) == 0 {
		return model.Snapshot{}, fmt.Errorf("arquivo não contém amostras")
	}
	return t.Samples[len(t.Samples)-1], nil
}
func LoadTrace(path string) (model.Trace, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return model.Trace{}, err
	}
	var t model.Trace
	if err := json.Unmarshal(b, &t); err == nil && len(t.Samples) > 0 {
		return t, nil
	}
	var s model.Snapshot
	if err := json.Unmarshal(b, &s); err != nil {
		return model.Trace{}, err
	}
	return model.Trace{Schema: s.Schema, StartedAt: s.CapturedAt, EndedAt: s.CapturedAt, Samples: []model.Snapshot{s}}, nil
}
