package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func u64(v uint64) *uint64   { return &v }
func f64(v float64) *float64 { return &v }

func TestMarshalFullyPopulatedSnapshot(t *testing.T) {
	s := Snapshot{
		System: SystemSnapshot{
			CPU:    f64(12.5),
			RAM:    &RAMInfo{Used: u64(8 << 30), Total: u64(16 << 30)},
			DiskIO: &DiskIORate{Read: f64(1e6), Write: f64(2e6)},
			NetIO:  &NetIORate{Rx: f64(3e6), Tx: f64(4e6)},
			Temps: []TemperatureSensor{
				{Label: "cpu_thermal", Value: f64(55.5)},
			},
			GPU: &GPUInfo{
				Busy:      f64(30.0),
				VRAMUsed:  u64(1 << 30),
				VRAMTotal: u64(8 << 30),
				Temp:      f64(45.0),
			},
		},
		Processes: []ProcessRow{
			{PID: 1, Name: "init", CPU: f64(0.1), RAM: &RAMInfo{Used: u64(1 << 20)}, DiskIO: &DiskIORate{Read: f64(1024), Write: f64(2048)}},
		},
		Docker: []ComposeGroup{
			{
				Project: "core",
				Containers: []ContainerRow{
					{ID: "abc123", Name: "core_app", CPU: f64(2.0), RAM: &RAMInfo{Used: u64(128 << 20)}},
				},
			},
		},
	}

	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	wantKeys := []string{
		"\"system\"", "\"cpu\"", "\"ram\"", "\"used\"", "\"total\"",
		"\"disk_io\"", "\"read\"", "\"write\"",
		"\"net_io\"", "\"rx\"", "\"tx\"",
		"\"temps\"", "\"label\"", "\"value\"",
		"\"gpu\"", "\"busy\"", "\"vram_used\"", "\"vram_total\"", "\"temp\"",
		"\"processes\"", "\"pid\"", "\"name\"",
		"\"docker\"", "\"project\"", "\"containers\"", "\"id\"",
	}
	for _, k := range wantKeys {
		if !strings.Contains(string(b), k) {
			t.Errorf("marshaled JSON missing key %s: %s", k, string(b))
		}
	}
}

func TestMarshalMetricsAbsentSnapshot(t *testing.T) {
	s := Snapshot{
		System: SystemSnapshot{
			CPU:    nil,
			RAM:    nil,
			DiskIO: nil,
			NetIO:  nil,
			Temps:  nil,
			GPU:    nil,
		},
		Processes: []ProcessRow{
			{PID: 1, Name: "init"},
		},
	}

	b, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	absentKeys := []string{"\"cpu\"", "\"ram\"", "\"disk_io\"", "\"net_io\"", "\"temps\"", "\"gpu\""}
	for _, k := range absentKeys {
		if strings.Contains(string(b), k) {
			t.Errorf("marshaled JSON should omit %s, got: %s", k, string(b))
		}
	}

	// Mandatory fields must still be present.
	mandatory := []string{"\"system\"", "\"processes\"", "\"pid\"", "\"name\""}
	for _, k := range mandatory {
		if !strings.Contains(string(b), k) {
			t.Errorf("marshaled JSON missing mandatory key %s: %s", k, string(b))
		}
	}
}

func TestProcessRowAbsentMetricsAreNullOrOmitted(t *testing.T) {
	p := ProcessRow{PID: 42, Name: "test"}
	b, _ := json.Marshal(p)
	if strings.Contains(string(b), "\"cpu\"") || strings.Contains(string(b), "\"ram\"") || strings.Contains(string(b), "\"disk_io\"") {
		t.Errorf("absent process metrics should be omitted: %s", string(b))
	}
}
