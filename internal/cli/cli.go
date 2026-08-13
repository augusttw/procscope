package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/augusttw/procscope/internal/analysis"
	term "github.com/augusttw/procscope/internal/format"
	"github.com/augusttw/procscope/internal/model"
	"github.com/augusttw/procscope/internal/observe"
	"github.com/augusttw/procscope/internal/procfs"
	"github.com/augusttw/procscope/internal/storage"
)

type App struct {
	Out, Err io.Writer
	Source   observe.Source
}

func New() *App { return &App{Out: os.Stdout, Err: os.Stderr, Source: procfs.New("")} }

func (a *App) Run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return a.help()
	}
	switch args[0] {
	case "help", "-h", "--help":
		return a.help()
	case "version", "--version":
		fmt.Fprintln(a.Out, "procscope dev")
		return nil
	case "attach":
		return a.attach(ctx, args[1:])
	case "run":
		return a.runProcess(ctx, args[1:])
	case "ports":
		return a.network(ctx, args[1:], true)
	case "connections":
		return a.network(ctx, args[1:], false)
	case "record":
		return a.record(ctx, args[1:])
	case "diff":
		return a.diff(args[1:])
	case "doctor":
		return a.doctor(ctx, args[1:])
	default:
		return fmt.Errorf("comando desconhecido %q; use procscope help", args[0])
	}
}

func (a *App) help() error {
	fmt.Fprint(a.Out, `procscope — observabilidade local para processos Linux

Uso:
  procscope run [opções] -- COMANDO [ARG...]
  procscope attach [opções] PID
  procscope ports PID
  procscope connections PID
  procscope record [opções] PID
  procscope diff ARQUIVO_A ARQUIVO_B
  procscope doctor [--file TRACE.json | PID]

Opções comuns de amostragem: --interval 1s, --duration 0 (até Ctrl-C)
`)
	return nil
}

func (a *App) attach(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("attach", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	interval := fs.Duration("interval", time.Second, "intervalo")
	duration := fs.Duration("duration", 0, "duração")
	once := fs.Bool("once", false, "uma única amostra")
	jsonOut := fs.Bool("json", false, "JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	pid, err := requiredPID(fs.Args())
	if err != nil {
		return err
	}
	if *once {
		return a.printOnce(ctx, pid, *jsonOut)
	}
	return a.stream(ctx, pid, *interval, *duration, nil)
}

func (a *App) runProcess(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	interval := fs.Duration("interval", time.Second, "intervalo")
	duration := fs.Duration("duration", 0, "duração máxima")
	if err := fs.Parse(args); err != nil {
		return err
	}
	command := fs.Args()
	if len(command) == 0 {
		return fmt.Errorf("informe o comando após --")
	}
	runCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	if *duration > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(runCtx, *duration)
		defer cancel()
	}
	cmd := exec.CommandContext(runCtx, command[0], command[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "procscope: executando PID %d (%s)\n", cmd.Process.Pid, strings.Join(command, " "))
	watchCtx, cancel := context.WithCancel(runCtx)
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
		cancel()
	}()
	watchErr := a.stream(watchCtx, cmd.Process.Pid, *interval, 0, cmd)
	waitErr := <-done
	if watchErr != nil && !processGone(watchErr) {
		return watchErr
	}
	if waitErr != nil {
		return fmt.Errorf("processo terminou: %w", waitErr)
	}
	return nil
}

func (a *App) stream(ctx context.Context, pid int, interval, duration time.Duration, cmd *exec.Cmd) error {
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if duration > 0 {
		var c context.CancelFunc
		ctx, c = context.WithTimeout(ctx, duration)
		defer c()
	}
	samples, errs := observe.Sampler{Source: a.Source, Interval: interval}.Stream(ctx, pid)
	for {
		select {
		case s, ok := <-samples:
			if !ok {
				return nil
			}
			term.Event(a.Out, s)
		case err := <-errs:
			if err != nil {
				return err
			}
			return nil
		case <-ctx.Done():
			return nil
		}
	}
}

func (a *App) printOnce(ctx context.Context, pid int, asJSON bool) error {
	s, err := a.Source.Snapshot(ctx, pid)
	if err != nil {
		return err
	}
	if asJSON {
		return json.NewEncoder(a.Out).Encode(s)
	}
	term.Snapshot(a.Out, s)
	return nil
}

func (a *App) network(ctx context.Context, args []string, ports bool) error {
	pid, err := requiredPID(args)
	if err != nil {
		return err
	}
	s, err := a.Source.Snapshot(ctx, pid)
	if err != nil {
		return err
	}
	for _, c := range s.Connections {
		if ports != (c.State == "LISTEN") {
			continue
		}
		if ports {
			fmt.Fprintf(a.Out, "%-5s %-24s %s\n", c.Protocol, c.Local, c.State)
		} else {
			fmt.Fprintf(a.Out, "%-5s %-24s -> %-24s %-13s %s\n", c.Protocol, c.Local, c.Remote, c.State, c.Dependency)
		}
	}
	return nil
}

func (a *App) record(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("record", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	interval := fs.Duration("interval", time.Second, "intervalo")
	duration := fs.Duration("duration", 10*time.Second, "duração")
	out := fs.String("output", "procscope-trace.json", "arquivo JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	pid, err := requiredPID(fs.Args())
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if *duration > 0 {
		var c context.CancelFunc
		ctx, c = context.WithTimeout(ctx, *duration)
		defer c()
	}
	trace := model.Trace{Schema: model.SchemaVersion, StartedAt: time.Now()}
	samples, errs := observe.Sampler{Source: a.Source, Interval: *interval}.Stream(ctx, pid)
loop:
	for {
		select {
		case s, ok := <-samples:
			if !ok {
				break loop
			}
			trace.Samples = append(trace.Samples, s)
			term.Event(a.Out, s)
		case err := <-errs:
			if err != nil && !processGone(err) {
				return err
			}
			break loop
		case <-ctx.Done():
			break loop
		}
	}
	if len(trace.Samples) == 0 {
		return fmt.Errorf("nenhuma amostra coletada")
	}
	trace.EndedAt = time.Now()
	if err := storage.Save(*out, trace); err != nil {
		return err
	}
	fmt.Fprintf(a.Out, "trace salvo em %s (%d amostras)\n", *out, len(trace.Samples))
	return nil
}

func (a *App) diff(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("uso: procscope diff ARQUIVO_A ARQUIVO_B")
	}
	x, err := storage.LoadSnapshot(args[0])
	if err != nil {
		return fmt.Errorf("%s: %w", args[0], err)
	}
	y, err := storage.LoadSnapshot(args[1])
	if err != nil {
		return fmt.Errorf("%s: %w", args[1], err)
	}
	d := analysis.Compare(x, y)
	fmt.Fprintln(a.Out, "MÉTRICA          ANTES          DEPOIS         DELTA")
	for _, c := range d.Changes {
		fmt.Fprintf(a.Out, "%-16s %14.2f %14.2f %+14.2f\n", c.Metric, c.Before, c.After, c.Delta)
	}
	for _, c := range d.Added {
		fmt.Fprintf(a.Out, "+ conexão %s %s -> %s (%s)\n", c.Protocol, c.Local, c.Remote, c.State)
	}
	for _, c := range d.Removed {
		fmt.Fprintf(a.Out, "- conexão %s %s -> %s (%s)\n", c.Protocol, c.Local, c.Remote, c.State)
	}
	return nil
}

func (a *App) doctor(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(a.Err)
	file := fs.String("file", "", "snapshot ou trace JSON")
	jsonOut := fs.Bool("json", false, "JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	var samples []model.Snapshot
	if *file != "" {
		t, err := storage.LoadTrace(*file)
		if err != nil {
			return err
		}
		samples = t.Samples
	} else {
		pid, err := requiredPID(fs.Args())
		if err != nil {
			return fmt.Errorf("informe PID ou --file: %w", err)
		}
		s, err := a.Source.Snapshot(ctx, pid)
		if err != nil {
			return err
		}
		samples = []model.Snapshot{s}
	}
	findings := analysis.Doctor(samples)
	if *jsonOut {
		return json.NewEncoder(a.Out).Encode(findings)
	}
	for _, f := range findings {
		fmt.Fprintf(a.Out, "[%-8s] %s: %s\n", strings.ToUpper(string(f.Severity)), f.Code, f.Message)
		if f.Suggestion != "" {
			fmt.Fprintf(a.Out, "           %s\n", f.Suggestion)
		}
	}
	return nil
}

func requiredPID(args []string) (int, error) {
	if len(args) != 1 {
		return 0, fmt.Errorf("um PID é obrigatório")
	}
	pid, err := strconv.Atoi(args[0])
	if err != nil || pid <= 0 {
		return 0, fmt.Errorf("PID inválido %q", args[0])
	}
	return pid, nil
}
func processGone(err error) bool {
	return errors.Is(err, os.ErrNotExist) || strings.Contains(err.Error(), "no such file")
}
