package boot

import (
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// moveInFixture is a machine's data path with content already on its disk, a
// volume directory for it and that volume's layout record.
func moveInFixture(t *testing.T) (target, volDir, record string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("a machine's wiring runs in its Linux guest")
	}
	root := t.TempDir()
	target = filepath.Join(root, "var", "lib", "docker")
	if err := os.MkdirAll(filepath.Join(target, "overlay2"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0o711); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "engine-id"), []byte("boxd\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return target, filepath.Join(root, ".discobox", "data", "var", "lib", "docker"), filepath.Join(root, ".discobox", "data", layoutsDir, "%2Fvar%2Flib%2Fdocker")
}

// A machine's existing content at a data path is moved into the path's
// volume, which takes the path's own mode, and the volume is recorded bound:
// the bind that follows shows what the box had there rather than hiding it,
// and nothing is overlaid (ADR 26-11-02-023 §1).
func TestAMachinesContentIsMovedIntoItsVolume(t *testing.T) {
	target, volDir, record := moveInFixture(t)
	b := &booter{run: func(name string, args ...string) error {
		t.Fatalf("ran %s %v; one filesystem is a rename", name, args)
		return nil
	}}
	if err := b.moveIn(target, volDir, record); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(volDir, "engine-id")); err != nil || string(data) != "boxd\n" {
		t.Fatalf("engine-id in the volume = %q, %v", data, err)
	}
	if info, err := os.Stat(filepath.Join(volDir, "overlay2")); err != nil || !info.IsDir() {
		t.Fatalf("overlay2 in the volume: %v", err)
	}
	if empty, err := dirEmpty(target); err != nil || !empty {
		t.Fatalf("the path still holds content (%v): it would be hidden under the bind", err)
	}
	if info, err := os.Stat(volDir); err != nil || info.Mode().Perm() != 0o711 {
		t.Fatalf("volume root = %v, %v; want the path's 0711", info.Mode(), err)
	}
	if layout, ok, err := readLayout(record); err != nil || !ok || layout != layoutBind {
		t.Fatalf("layout record = %q %v %v, want bind", layout, ok, err)
	}

	// Recorded, it moves nothing again: what the box's services write under
	// the bound path is the volume's, and what is under the mountpoint at the
	// next boot is not the box's to take.
	if err := os.WriteFile(filepath.Join(target, "later"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := b.moveIn(target, volDir, record); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(volDir, "later")); !os.IsNotExist(err) {
		t.Fatalf("a recorded volume took more of the path: %v", err)
	}
}

// A boot cut short part way through leaves no record, and the next moves the
// rest; a name already in the volume is refused rather than overwritten.
func TestAMachinesMoveInResumesAndOverwritesNothing(t *testing.T) {
	target, volDir, record := moveInFixture(t)
	if err := os.MkdirAll(volDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(target, "overlay2"), filepath.Join(volDir, "overlay2")); err != nil {
		t.Fatal(err)
	}
	b := &booter{}
	if err := b.moveIn(target, volDir, record); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(volDir, "engine-id")); err != nil {
		t.Fatalf("the rest was not moved: %v", err)
	}

	target, volDir, record = moveInFixture(t)
	if err := os.MkdirAll(volDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(volDir, "engine-id"), []byte("mine\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := b.moveIn(target, volDir, record)
	if err == nil || !strings.Contains(err.Error(), "is there already") {
		t.Fatalf("move over a name in the volume = %v, want it refused", err)
	}
	if data, _ := os.ReadFile(filepath.Join(volDir, "engine-id")); string(data) != "mine\n" {
		t.Fatalf("the volume's engine-id = %q, overwritten", data)
	}
	if _, ok, _ := readLayout(record); ok {
		t.Fatal("recorded a volume whose move failed")
	}
}

// A volume recorded as an overlay is not a machine's: it is refused rather
// than bound, which would show its upper and work directories as the path.
func TestAMachineRefusesAnOverlaidVolume(t *testing.T) {
	target, volDir, record := moveInFixture(t)
	if err := writeLayout(record, layoutOverlay); err != nil {
		t.Fatal(err)
	}
	if err := (&booter{}).moveIn(target, volDir, record); err == nil {
		t.Fatal("moved into a volume recorded as an overlay")
	}
}

// Provision does nothing on a boot Wire ran on, a machine's as a
// container's: the marker Wire leaves in /run says so.
func TestProvisionDoesNothingOnABootWireRan(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Provision provisions a Linux guest alone")
	}
	dir := t.TempDir()
	useRecords(t, dir)
	saved := wiredMarker
	wiredMarker = filepath.Join(dir, "run", "wired")
	t.Cleanup(func() { wiredMarker = saved })
	if err := markWired(); err != nil {
		t.Fatal(err)
	}
	// A bootstrap Provision could not provision, were it to try.
	bootstrap := writeBootstrap(t, dir, "")
	if err := Provision(slog.Default(), bootstrap); err != nil {
		t.Fatalf("provision after Wire = %v, want nothing done", err)
	}
	if err := os.Remove(wiredMarker); err != nil {
		t.Fatal(err)
	}
	if err := Provision(slog.Default(), bootstrap); err == nil {
		t.Fatal("provision with no marker did nothing; it should have read the bootstrap")
	}
}
