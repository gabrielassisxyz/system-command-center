// Package gpu reads AMD GPU metrics from sysfs.
//
// It is absent-tolerant: any missing file leaves the corresponding field absent
// rather than returning an error. This matches the project-wide rule that missing
// metrics are represented as nil in the model.
package gpu

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"hardware-usage/internal/model"
)

// SysfsRoot is the base directory used to discover AMD GPU sysfs files. Tests
// inject a fixture directory instead of the real /sys.
type SysfsRoot string

// defaultSysfsRoot is the production sysfs path.
const defaultSysfsRoot SysfsRoot = "/sys"

// Reader discovers the amdgpu sysfs entries and reads the metrics the UI
// needs: GPU busy percent, VRAM used/total, and a single GPU temperature.
type Reader struct {
	root SysfsRoot
}

// NewReader returns a Reader that reads from the production /sys filesystem.
func NewReader() *Reader {
	return &Reader{root: defaultSysfsRoot}
}

// NewReaderWithRoot returns a Reader rooted at root. Tests use this to point
// the reader at a fixture directory.
func NewReaderWithRoot(root SysfsRoot) *Reader {
	return &Reader{root: root}
}

// Read returns the AMD GPU snapshot for the current machine. If any file is
// missing or cannot be parsed, the corresponding model field is left nil.
func (r *Reader) Read(_ context.Context) model.GPUInfo {
	var info model.GPUInfo

	drm := filepath.Join(string(r.root), "class", "drm")
	card, ok := findCardDevice(drm)
	if !ok {
		return info
	}
	deviceDir := filepath.Join(drm, card, "device")

	if v, ok := readUint(filepath.Join(deviceDir, "gpu_busy_percent")); ok {
		f := float64(v)
		info.Busy = &f
	}

	if v, ok := readUint(filepath.Join(deviceDir, "mem_info_vram_used")); ok {
		info.VRAMUsed = &v
	}
	if v, ok := readUint(filepath.Join(deviceDir, "mem_info_vram_total")); ok {
		info.VRAMTotal = &v
	}

	if t, ok := readHWMontemp(deviceDir); ok {
		info.Temp = &t
	}

	return info
}

// findCardDevice scans /sys/class/drm for a cardN entry whose device is an
// amdgpu. It returns the first matching card name, e.g. "card1". If none is
// found, ok is false.
func findCardDevice(drm string) (string, bool) {
	entries, err := os.ReadDir(drm)
	if err != nil {
		return "", false
	}
	for _, e := range entries {
		if !e.IsDir() && e.Type()&os.ModeSymlink == 0 {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "card") || strings.Contains(name, "-") {
			continue
		}
		deviceDir := filepath.Join(drm, name, "device")
		if !isDir(deviceDir) {
			continue
		}
		if vendor := strings.TrimSpace(readString(filepath.Join(deviceDir, "vendor"))); vendor != "0x1002" {
			continue
		}
		return name, true
	}
	return "", false
}

// readHWMontemp looks for an amdgpu hwmon directory associated with deviceDir
// and returns its first temperature input (temp1_input) converted to degrees
// Celsius. hwmon temp files report millidegrees, so we divide by 1000.
func readHWMontemp(deviceDir string) (float64, bool) {
	hwmonDir := filepath.Join(deviceDir, "hwmon")
	entries, err := os.ReadDir(hwmonDir)
	if err != nil {
		return 0, false
	}
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "hwmon") {
			continue
		}
		d := filepath.Join(hwmonDir, e.Name())
		if name := strings.TrimSpace(readString(filepath.Join(d, "name"))); name != "amdgpu" {
			continue
		}
		return readMillidegrees(filepath.Join(d, "temp1_input"))
	}
	return 0, false
}

func readUint(path string) (uint64, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	v, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func readString(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

func readMillidegrees(path string) (float64, bool) {
	v, ok := readUint(path)
	if !ok {
		return 0, false
	}
	return float64(v) / 1000.0, true
}

func isDir(path string) bool {
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}
	return fi.IsDir()
}
