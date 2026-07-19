package gpu

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"hardware-usage/internal/model"
)

// makeCardFixture creates a minimal sysfs fixture with a single amdgpu card.
// It returns the sysfs root and the card name.
func makeCardFixture(t *testing.T, card string) SysfsRoot {
	t.Helper()
	root := t.TempDir()
	cardDir := filepath.Join(root, "class", "drm", card)
	devDir := filepath.Join(cardDir, "device")
	if err := os.MkdirAll(devDir, 0755); err != nil {
		t.Fatalf("create device dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(devDir, "vendor"), []byte("0x1002\n"), 0644); err != nil {
		t.Fatalf("write vendor: %v", err)
	}
	return SysfsRoot(root)
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestReader_FullyPopulated(t *testing.T) {
	root := makeCardFixture(t, "card1")
	devDir := filepath.Join(string(root), "class", "drm", "card1", "device")
	writeFile(t, filepath.Join(devDir, "gpu_busy_percent"), []byte("45\n"))
	writeFile(t, filepath.Join(devDir, "mem_info_vram_used"), []byte("1073741824\n"))
	writeFile(t, filepath.Join(devDir, "mem_info_vram_total"), []byte("8589934592\n"))

	hwmonDir := filepath.Join(devDir, "hwmon", "hwmon1")
	if err := os.MkdirAll(hwmonDir, 0755); err != nil {
		t.Fatalf("create hwmon dir: %v", err)
	}
	writeFile(t, filepath.Join(hwmonDir, "name"), []byte("amdgpu\n"))
	writeFile(t, filepath.Join(hwmonDir, "temp1_input"), []byte("55000\n"))

	reader := NewReaderWithRoot(root)
	info := reader.Read(context.Background())

	if info.Busy == nil || *info.Busy != 45.0 {
		t.Errorf("Busy = %v, want 45.0", info.Busy)
	}
	if info.VRAMUsed == nil || *info.VRAMUsed != 1073741824 {
		t.Errorf("VRAMUsed = %v, want 1073741824", info.VRAMUsed)
	}
	if info.VRAMTotal == nil || *info.VRAMTotal != 8589934592 {
		t.Errorf("VRAMTotal = %v, want 8589934592", info.VRAMTotal)
	}
	if info.Temp == nil || *info.Temp != 55.0 {
		t.Errorf("Temp = %v, want 55.0", info.Temp)
	}
}

func TestReader_MissingFiles_ReturnsAbsentFields(t *testing.T) {
	root := makeCardFixture(t, "card0")
	// No metric files written at all.

	reader := NewReaderWithRoot(root)
	info := reader.Read(context.Background())

	if info.Busy != nil {
		t.Errorf("Busy should be absent, got %v", *info.Busy)
	}
	if info.VRAMUsed != nil {
		t.Errorf("VRAMUsed should be absent, got %v", *info.VRAMUsed)
	}
	if info.VRAMTotal != nil {
		t.Errorf("VRAMTotal should be absent, got %v", *info.VRAMTotal)
	}
	if info.Temp != nil {
		t.Errorf("Temp should be absent, got %v", *info.Temp)
	}
}

func TestReader_PartialFiles_ReturnsPresentFieldsOnly(t *testing.T) {
	root := makeCardFixture(t, "card1")
	devDir := filepath.Join(string(root), "class", "drm", "card1", "device")
	writeFile(t, filepath.Join(devDir, "gpu_busy_percent"), []byte("12\n"))
	// VRAM files and hwmon intentionally missing.

	reader := NewReaderWithRoot(root)
	info := reader.Read(context.Background())

	if info.Busy == nil || *info.Busy != 12.0 {
		t.Errorf("Busy = %v, want 12.0", info.Busy)
	}
	if info.VRAMUsed != nil || info.VRAMTotal != nil || info.Temp != nil {
		t.Errorf("expected VRAM and temp fields to be absent, got %+v", info)
	}
}

func TestReader_IgnoresNonAMDGPU(t *testing.T) {
	root := t.TempDir()
	cardDir := filepath.Join(root, "class", "drm", "card0")
	devDir := filepath.Join(cardDir, "device")
	if err := os.MkdirAll(devDir, 0755); err != nil {
		t.Fatalf("create device dir: %v", err)
	}
	writeFile(t, filepath.Join(devDir, "vendor"), []byte("0x10de\n"))
	writeFile(t, filepath.Join(devDir, "gpu_busy_percent"), []byte("99\n"))

	reader := NewReaderWithRoot(SysfsRoot(root))
	info := reader.Read(context.Background())

	if info.Busy != nil {
		t.Errorf("expected no matching amdgpu card, got Busy=%v", *info.Busy)
	}
}

func TestReader_PicksFirstAMDGPUCard(t *testing.T) {
	root := t.TempDir()
	for _, card := range []string{"card0", "card1"} {
		cardDir := filepath.Join(root, "class", "drm", card)
		devDir := filepath.Join(cardDir, "device")
		if err := os.MkdirAll(devDir, 0755); err != nil {
			t.Fatalf("create %s device dir: %v", card, err)
		}
		writeFile(t, filepath.Join(devDir, "vendor"), []byte("0x1002\n"))
	}

	devDir := filepath.Join(root, "class", "drm", "card0", "device")
	writeFile(t, filepath.Join(devDir, "gpu_busy_percent"), []byte("7\n"))
	devDir1 := filepath.Join(root, "class", "drm", "card1", "device")
	writeFile(t, filepath.Join(devDir1, "gpu_busy_percent"), []byte("8\n"))

	reader := NewReaderWithRoot(SysfsRoot(root))
	info := reader.Read(context.Background())

	if info.Busy == nil || *info.Busy != 7.0 {
		t.Errorf("Busy = %v, want first amdgpu card value 7.0", info.Busy)
	}
}

func TestReader_HwmonWithWrongName_Ignored(t *testing.T) {
	root := makeCardFixture(t, "card1")
	devDir := filepath.Join(string(root), "class", "drm", "card1", "device")
	writeFile(t, filepath.Join(devDir, "gpu_busy_percent"), []byte("30\n"))

	hwmonDir := filepath.Join(devDir, "hwmon", "hwmon1")
	if err := os.MkdirAll(hwmonDir, 0755); err != nil {
		t.Fatalf("create hwmon dir: %v", err)
	}
	writeFile(t, filepath.Join(hwmonDir, "name"), []byte("k10temp\n"))
	writeFile(t, filepath.Join(hwmonDir, "temp1_input"), []byte("45000\n"))

	reader := NewReaderWithRoot(root)
	info := reader.Read(context.Background())

	if info.Temp != nil {
		t.Errorf("expected temp absent when hwmon name is not amdgpu, got %v", *info.Temp)
	}
}

func TestReader_ZeroBusy_IsStillPresent(t *testing.T) {
	root := makeCardFixture(t, "card1")
	devDir := filepath.Join(string(root), "class", "drm", "card1", "device")
	writeFile(t, filepath.Join(devDir, "gpu_busy_percent"), []byte("0\n"))

	reader := NewReaderWithRoot(root)
	info := reader.Read(context.Background())

	if info.Busy == nil || *info.Busy != 0.0 {
		t.Errorf("Busy = %v, want 0.0 (zero is a valid metric)", info.Busy)
	}
}

// Compile-time check that Reader produces the expected model type.
var _ = model.GPUInfo{}
