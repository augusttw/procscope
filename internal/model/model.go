package model

import "time"

const SchemaVersion = 1

type Process struct {
	PID        int           `json:"pid"`
	Name       string        `json:"name"`
	Command    []string      `json:"command,omitempty"`
	State      string        `json:"state"`
	Threads    int           `json:"threads"`
	CPUPercent float64       `json:"cpu_percent"`
	RSSBytes   uint64        `json:"rss_bytes"`
	VMSBytes   uint64        `json:"vm_bytes"`
	ReadBytes  uint64        `json:"read_bytes"`
	WriteBytes uint64        `json:"write_bytes"`
	OpenFDs    int           `json:"open_fds"`
	Uptime     time.Duration `json:"uptime"`
}

type Connection struct {
	Protocol   string `json:"protocol"`
	Local      string `json:"local"`
	Remote     string `json:"remote,omitempty"`
	State      string `json:"state"`
	Inode      uint64 `json:"inode,omitempty"`
	Dependency string `json:"dependency,omitempty"`
}

type Snapshot struct {
	Schema      int          `json:"schema"`
	CapturedAt  time.Time    `json:"captured_at"`
	Process     Process      `json:"process"`
	Connections []Connection `json:"connections,omitempty"`
}

type Trace struct {
	Schema    int        `json:"schema"`
	StartedAt time.Time  `json:"started_at"`
	EndedAt   time.Time  `json:"ended_at"`
	Samples   []Snapshot `json:"samples"`
}
