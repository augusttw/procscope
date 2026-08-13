package procfs

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/augusttw/procscope/internal/model"
)

type Source struct {
	Root     string
	mu       sync.Mutex
	previous map[int]cpuSample
}

type cpuSample struct {
	at    time.Time
	ticks uint64
}

func New(root string) *Source {
	if root == "" {
		root = "/proc"
	}
	return &Source{Root: root, previous: make(map[int]cpuSample)}
}

func (s *Source) Snapshot(ctx context.Context, pid int) (model.Snapshot, error) {
	select {
	case <-ctx.Done():
		return model.Snapshot{}, ctx.Err()
	default:
	}
	p, err := s.readProcess(pid)
	if err != nil {
		return model.Snapshot{}, err
	}
	c, err := s.readConnections(pid)
	if err != nil && !errors.Is(err, os.ErrPermission) {
		return model.Snapshot{}, err
	}
	return model.Snapshot{Schema: model.SchemaVersion, CapturedAt: time.Now(), Process: p, Connections: c}, nil
}

func (s *Source) readProcess(pid int) (model.Process, error) {
	base := filepath.Join(s.Root, strconv.Itoa(pid))
	statRaw, err := os.ReadFile(filepath.Join(base, "stat"))
	if err != nil {
		return model.Process{}, fmt.Errorf("processo %d: %w", pid, err)
	}
	stat, err := parseStat(string(statRaw))
	if err != nil {
		return model.Process{}, err
	}
	status := readKeyValues(filepath.Join(base, "status"))
	ioValues := readKeyValues(filepath.Join(base, "io"))
	cmdRaw, _ := os.ReadFile(filepath.Join(base, "cmdline"))
	fds, _ := os.ReadDir(filepath.Join(base, "fd"))
	pageSize := uint64(os.Getpagesize())
	boot, bootErr := s.bootTime()
	var uptime time.Duration
	if bootErr == nil {
		uptime = time.Since(boot.Add(time.Duration(float64(stat.startTicks)/clockTicks()) * time.Second))
		if uptime < 0 {
			uptime = 0
		}
	}
	return model.Process{
		PID: pid, Name: stat.name, Command: splitNull(cmdRaw), State: stateName(stat.state),
		Threads: intValue(status["Threads"]), CPUPercent: s.cpuPercent(pid, stat.userTicks+stat.systemTicks),
		RSSBytes: uint64(max64(stat.rssPages, 0)) * pageSize, VMSBytes: stat.vmBytes,
		ReadBytes: uintValue(ioValues["read_bytes"]), WriteBytes: uintValue(ioValues["write_bytes"]),
		OpenFDs: len(fds), Uptime: uptime,
	}, nil
}

type processStat struct {
	name, state                                 string
	userTicks, systemTicks, startTicks, vmBytes uint64
	rssPages                                    int64
}

func parseStat(raw string) (processStat, error) {
	left, right := strings.Index(raw, "("), strings.LastIndex(raw, ")")
	if left < 0 || right <= left {
		return processStat{}, fmt.Errorf("/proc stat inválido")
	}
	f := strings.Fields(strings.TrimSpace(raw[right+1:]))
	if len(f) < 22 {
		return processStat{}, fmt.Errorf("/proc stat incompleto")
	}
	return processStat{name: raw[left+1 : right], state: f[0], userTicks: parseUint(f[11]), systemTicks: parseUint(f[12]), startTicks: parseUint(f[19]), vmBytes: parseUint(f[20]), rssPages: parseInt64(f[21])}, nil
}

func (s *Source) cpuPercent(pid int, ticks uint64) float64 {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.previous[pid]
	s.previous[pid] = cpuSample{now, ticks}
	if !ok || ticks < old.ticks {
		return 0
	}
	seconds := now.Sub(old.at).Seconds()
	if seconds <= 0 {
		return 0
	}
	return float64(ticks-old.ticks) / clockTicks() / seconds * 100
}

func (s *Source) bootTime() (time.Time, error) {
	f, err := os.Open(filepath.Join(s.Root, "stat"))
	if err != nil {
		return time.Time{}, err
	}
	defer f.Close()
	scan := bufio.NewScanner(f)
	for scan.Scan() {
		if strings.HasPrefix(scan.Text(), "btime ") {
			sec, _ := strconv.ParseInt(strings.TrimSpace(strings.TrimPrefix(scan.Text(), "btime ")), 10, 64)
			return time.Unix(sec, 0), nil
		}
	}
	return time.Time{}, fmt.Errorf("btime ausente")
}

func clockTicks() float64 { return 100 } // Linux commonly exposes USER_HZ=100 for /proc.

func readKeyValues(path string) map[string]string {
	r := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		return r
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		p := strings.SplitN(s.Text(), ":", 2)
		if len(p) == 2 {
			values := strings.Fields(p[1])
			if len(values) > 0 {
				r[p[0]] = values[0]
			}
		}
	}
	return r
}

func (s *Source) readConnections(pid int) ([]model.Connection, error) {
	inodes := map[uint64]bool{}
	fds, err := os.ReadDir(filepath.Join(s.Root, strconv.Itoa(pid), "fd"))
	if err != nil {
		return nil, err
	}
	for _, fd := range fds {
		target, err := os.Readlink(filepath.Join(s.Root, strconv.Itoa(pid), "fd", fd.Name()))
		if err != nil {
			continue
		}
		if strings.HasPrefix(target, "socket:[") {
			n := strings.TrimSuffix(strings.TrimPrefix(target, "socket:["), "]")
			inodes[parseUint(n)] = true
		}
	}
	var all []model.Connection
	for _, spec := range []struct{ file, proto string }{{"tcp", "tcp4"}, {"tcp6", "tcp6"}} {
		rows, err := parseNetFile(filepath.Join(s.Root, strconv.Itoa(pid), "net", spec.file), spec.proto, inodes)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		all = append(all, rows...)
	}
	return all, nil
}

func parseNetFile(path, proto string, owned map[uint64]bool) ([]model.Connection, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []model.Connection
	scan := bufio.NewScanner(f)
	first := true
	for scan.Scan() {
		if first {
			first = false
			continue
		}
		p := strings.Fields(scan.Text())
		if len(p) < 10 {
			continue
		}
		inode := parseUint(p[9])
		if !owned[inode] {
			continue
		}
		local, e1 := decodeAddress(p[1], proto == "tcp6")
		remote, e2 := decodeAddress(p[2], proto == "tcp6")
		if e1 != nil || e2 != nil {
			continue
		}
		state := tcpState(p[3])
		dep := ""
		if state != "LISTEN" {
			dep = dependency(remote)
		}
		out = append(out, model.Connection{Protocol: proto, Local: local, Remote: remote, State: state, Inode: inode, Dependency: dep})
	}
	return out, scan.Err()
}

func decodeAddress(raw string, ipv6 bool) (string, error) {
	p := strings.Split(raw, ":")
	if len(p) != 2 {
		return "", fmt.Errorf("endereço inválido")
	}
	port, err := strconv.ParseUint(p[1], 16, 16)
	if err != nil {
		return "", err
	}
	b, err := strconv.ParseUint(p[0], 16, 32)
	if err != nil && !ipv6 {
		return "", err
	}
	if !ipv6 {
		ip := net.IPv4(byte(b), byte(b>>8), byte(b>>16), byte(b>>24))
		return net.JoinHostPort(ip.String(), strconv.Itoa(int(port))), nil
	}
	hex := p[0]
	if len(hex) != 32 {
		return "", fmt.Errorf("ipv6 inválido")
	}
	bytes := make([]byte, 16)
	for group := 0; group < 4; group++ {
		word, _ := strconv.ParseUint(hex[group*8:(group+1)*8], 16, 32)
		for j := 0; j < 4; j++ {
			bytes[group*4+j] = byte(word >> (8 * j))
		}
	}
	return net.JoinHostPort(net.IP(bytes).String(), strconv.Itoa(int(port))), nil
}

func dependency(addr string) string {
	_, p, err := net.SplitHostPort(addr)
	if err != nil {
		return ""
	}
	switch p {
	case "5432":
		return "postgresql"
	case "6379":
		return "redis"
	case "80", "443", "3000", "8000", "8080", "8443":
		return "http"
	}
	return ""
}
func tcpState(s string) string {
	states := map[string]string{"01": "ESTABLISHED", "02": "SYN_SENT", "03": "SYN_RECV", "04": "FIN_WAIT1", "05": "FIN_WAIT2", "06": "TIME_WAIT", "07": "CLOSE", "08": "CLOSE_WAIT", "09": "LAST_ACK", "0A": "LISTEN", "0B": "CLOSING"}
	if v := states[s]; v != "" {
		return v
	}
	return s
}
func stateName(s string) string {
	m := map[string]string{"R": "running", "S": "sleeping", "D": "disk sleep", "Z": "zombie", "T": "stopped", "I": "idle"}
	if v := m[s]; v != "" {
		return v
	}
	return s
}
func splitNull(b []byte) []string {
	var out []string
	for _, v := range strings.Split(strings.TrimRight(string(b), "\x00"), "\x00") {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}
func parseUint(v string) uint64 { n, _ := strconv.ParseUint(v, 10, 64); return n }
func parseInt64(v string) int64 { n, _ := strconv.ParseInt(v, 10, 64); return n }
func uintValue(v string) uint64 {
	f := strings.Fields(v)
	if len(f) == 0 {
		return 0
	}
	return parseUint(f[0])
}
func intValue(v string) int {
	f := strings.Fields(v)
	if len(f) == 0 {
		return 0
	}
	n, _ := strconv.Atoi(f[0])
	return n
}
func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
