package boot

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/discobox-ai/discobox/pkg/sandboxconfig"
	"github.com/discobox-ai/discobox/sandbox-agent/desktop"
	"github.com/discobox-ai/discobox/sandbox-agent/provision"
)

// Wire is a sandbox's boot: discobox-wire.service runs it from the box's own
// systemd, before local-fs.target and so before every unit that uses a wired
// path, in a pool's container (ADR 26-10-10-767) and on a machine (ADR
// 26-11-02-023 §2). It provisions the sandbox user from the bootstrap's own
// `user`, wires the image-declared volumes from the box's data and cache
// roots (ADR 0007), writes the units bound to the sandbox user, and reloads
// the manager so they are read. In a container it also binds the config
// volume onto /etc/discobox and the secrets volume onto /run/discobox/secrets,
// and wires the sources, all from the primary volumes the pool agent mounted.
//
// A machine has none of those volumes: its bootstrap is the one its pool
// placed at /etc/discobox, its sources and secrets arrive through the intake,
// and its data and cache roots are directories on its own disk, made here
// (ADR 26-11-02-023 §1).
//
// It runs on every boot, unlike Provision: neither a container's mounts nor a
// machine's survive a restart, nor a container's account a rebuild.
//
// An error fails the unit, and the unit's failure ends the container or
// powers the machine off: a sandbox whose wiring failed would write its state
// where nothing keeps it, the image layer, or under the paths it was meant to
// be bound over.
func Wire(logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}
	b := newBooter()
	// Only a pool's container has the config volume. Its copy is read
	// before it is bound onto /etc/discobox (ADR 0012 §6); a machine's
	// bootstrap is already there.
	path := manifestPath
	if !dirExists(configMountPath) {
		path, b.ownDisk = machineManifestPath, true
		for _, root := range []string{dataMountPath, cacheMountPath} {
			if err := os.MkdirAll(root, 0o755); err != nil {
				return err
			}
		}
	}
	bootstrap, err := loadEffectiveConfig(path)
	if err != nil {
		return err
	}
	id, err := bootstrapIdentity(bootstrap)
	if err != nil {
		return err
	}
	if err := b.provision(logger, id, &bootstrap, true); err != nil {
		return err
	}
	if err := wireSecrets(); err != nil {
		return fmt.Errorf("wire secrets: %w", err)
	}
	if err := b.reloadUserUnits(logger, id); err != nil {
		return err
	}
	return markWired()
}

// Provision is Wire's provisioning for a sandbox Wire did not run on this
// boot: a Linux box whose init does not run discobox-wire.service, where
// the agent is one of its services, started once the bootstrap at path has
// been placed (ADR 26-10-09-143 §1). It sets up the sandbox user and its
// groups, the working root, the home skeleton, ~/.gitconfig, direnv config
// and the units bound to the sandbox user, and wires nothing: the box is
// already running, and a mount laid under its running processes would split
// their view of the path (ADR 26-11-02-023, Alternatives rejected); sources
// arrive through the intake.
//
// Where Wire ran this boot, it has already done all of this, and Provision
// does nothing: Wire's marker in /run says so, which a restart clears.
//
// Only a Linux guest has accounts to provision. A macOS or Windows sandbox has
// one account, the image's own (ADR 0145 §5), and nothing here applies to it.
func Provision(logger *slog.Logger, path string) error {
	if logger == nil {
		logger = slog.Default()
	}
	if fileExists(wiredMarker) || runtime.GOOS != "linux" {
		return nil
	}
	return newBooter().provisionOnce(logger, path)
}

// wiredMarker says Wire ran on this boot. It is in /run, a tmpfs, so a
// restart clears it. A variable so a test does not read the box's own.
var wiredMarker = "/run/discobox/wired"

func markWired() error {
	if err := os.MkdirAll(filepath.Dir(wiredMarker), 0o755); err != nil {
		return err
	}
	return os.WriteFile(wiredMarker, nil, 0o600)
}

// JoinFeatureGroups wires the features of the box's bundle trees and joins
// the sandbox user to the groups its bootstrap names that they made. A box
// delivered its platform has no trees when Wire or Provision runs at its
// first boot — the agent fetches them after (ADR 26-10-16-423 §7) — so the
// groups only a feature creates (`brew`'s) were not there for the account to
// join. The agent calls this once its trees are in place and before
// readiness, which every session waits for, so the account has them before
// any session starts (§5).
func JoinFeatureGroups(logger *slog.Logger, path string) error {
	if logger == nil {
		logger = slog.Default()
	}
	if runtime.GOOS != "linux" {
		return nil
	}
	bootstrap, err := loadEffectiveConfig(path)
	if err != nil {
		return err
	}
	id, err := bootstrapIdentity(bootstrap)
	if err != nil {
		return err
	}
	b := newBooter()
	b.wireFeatures(logger, bootstrap.Features)
	return b.ensureAdditionalGroups(id, bootstrap.SandboxGroups())
}

// provisionOnce is Provision past its two guards.
//
// It runs once per sandbox, not on every start. The agent's start is not a
// boot: a restart after a crash finds the sandbox user's processes running,
// and re-running account changes and re-seeding under them is a second boot
// they did not ask for. The box's provisioning records (provision.Records)
// keep whose sandbox this disk was provisioned for, on the disk itself, so a
// stopped and restarted machine is not provisioned again either.
func (b *booter) provisionOnce(logger *slog.Logger, path string) error {
	bootstrap, err := loadEffectiveConfig(path)
	if err != nil {
		return err
	}
	if bootstrap.SandboxID == "" {
		return fmt.Errorf("bootstrap %s names no sandbox", path)
	}
	records := provision.Records{Dir: recordsDir}
	if records.Sandbox() == bootstrap.SandboxID {
		return nil
	}
	// A disk provisioned before the records were kept says so with the
	// marker that preceded them; it is carried into the records once.
	if provisionedFor(legacyProvisionedPath) == bootstrap.SandboxID {
		if err := records.RecordSandbox(bootstrap.SandboxID); err != nil {
			return err
		}
		return removeLegacyMarker(legacyProvisionedPath)
	}
	id, err := bootstrapIdentity(bootstrap)
	if err != nil {
		return err
	}
	if err := b.provision(logger, id, &bootstrap, false); err != nil {
		return err
	}
	// The agent's start is long after boot, unlike Wire's: the session bus's
	// socket has been listening since then, owned by root, so it is recreated
	// for the sandbox user. Nothing has connected to it yet: every client is a
	// process the agent starts. The proxy-trust environment the daemons read is
	// rendered from the bootstrap, so it was skipped at this boot and is
	// rendered now. Every later boot finds the drop-ins and the bootstrap on
	// disk and needs none of this. Both units belong to features a box may not
	// have (the session bus is the desktop's), so each is touched only where
	// its file is installed: systemctl fails on a unit it cannot find.
	if dirExists("/run/systemd/system") {
		if err := b.reloadUserUnits(logger, id); err != nil {
			return err
		}
		if fileExists("/etc/systemd/system/discobox-session-bus.socket") {
			if err := b.run("systemctl", "try-restart", "discobox-session-bus.socket"); err != nil {
				return err
			}
		}
		if fileExists("/etc/systemd/system/discobox-render-proxy-env.service") {
			if err := b.run("systemctl", "start", "discobox-render-proxy-env.service"); err != nil {
				return err
			}
		}
	}
	if err := records.RecordSandbox(bootstrap.SandboxID); err != nil {
		return err
	}
	return removeLegacyMarker(legacyProvisionedPath)
}

// bootstrapIdentity is who the sandbox runs as, from the bootstrap's own user:
// a unit does not see the container's DISCOBOX_USER_* (Wire), and a VM guest
// is given none (Provision), while sandbox.json's `user` is the same manifest
// layer that environment is rendered from (ADR 26-10-10-767 §3).
func bootstrapIdentity(bootstrap sandboxconfig.Config) (identity, error) {
	if err := bootstrap.User.Validate(); err != nil {
		return identity{}, fmt.Errorf("the manifest's user: %w", err)
	}
	id, err := resolveIdentity(&bootstrap.User)
	if err != nil {
		return identity{}, fmt.Errorf("resolve sandbox identity: %w", err)
	}
	return id, nil
}

// The features whose wiring boot does, skipped when provisioning reports one
// off. A feature the box does not report is its image's, and is wired.
const (
	featureDirenv  = "direnv"
	featureDesktop = "desktop"
)

// recordsDir holds the box's provisioning records, the sandbox phase's among
// them (ADR 26-10-16-423 §3). A variable so a test does not write the
// machine's own.
var recordsDir = provision.RecordsDir

// legacyProvisionedPath is the marker that named the sandbox a disk was
// provisioned for before the records did. A variable so a test does not read
// the machine's own.
var legacyProvisionedPath = "/var/lib/discobox/provisioned"

// provisionedFor is the sandbox ID the legacy marker at path records, or ""
// when it records none.
func provisionedFor(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func removeLegacyMarker(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// writeUnitDropins binds the desktop feature's per-user units -- the session
// bus and the desktop -- to the sandbox user, installed yet or not, and seeds
// the desktop's starting scale. A desktop turned off gets none of them: the
// session bus is the desktop's (ADR 26-09-25-146).
func writeUnitDropins(logger *slog.Logger, id identity, desktopOff bool) error {
	if desktopOff {
		return nil
	}
	if err := writeDesktopDropins(id); err != nil {
		return fmt.Errorf("desktop: %w", err)
	}
	if err := writeSessionBusDropins(id); err != nil {
		return fmt.Errorf("session bus: %w", err)
	}
	// Not fatal. Everything that reads the file tolerates its absence, and a
	// sandbox must not fail to start over a HiDPI hint.
	if err := seedDesktopScale(id); err != nil {
		logger.Warn("seed desktop scale", "error", err)
	}
	return nil
}

// reloadUserUnits writes the units bound to the sandbox user and has systemd,
// already running wherever this is called, read them.
func (b *booter) reloadUserUnits(logger *slog.Logger, id identity) error {
	if err := writeUnitDropins(logger, id, b.off(featureDesktop)); err != nil {
		return fmt.Errorf("write unit drop-ins: %w", err)
	}
	return b.run("systemctl", "daemon-reload")
}

// provision does the user setup and what the bootstrap declares for the user:
// groups, the working root, ~/.gitconfig, direnv config and desktop
// launchers. wire says the pool agent mounted the primary volumes (Wire, in a
// pool's container), and only then are the config volume, the declared volumes
// and the sources wired from them.
func (b *booter) provision(logger *slog.Logger, id identity, bootstrap *sandboxconfig.Config, wire bool) error {
	// A skills source's target may be home-relative (ADR 26-11-01-692 §3):
	// expanded here against the home boot resolved, so the direnv whitelist
	// and the wiring below see the place it is mounted.
	expanded := bootstrap.ExpandHome(sandboxPaths, id.home)
	bootstrap = &expanded
	// The features' groups and env go first, before the account exists, so
	// the account joins groups already there (ADR 26-10-16-423 §5); it also
	// decides which features are off, which the steps below read.
	if b.wireFeatures != nil {
		b.wireFeatures(logger, bootstrap.Features)
	}
	if err := b.ensureUser(id); err != nil {
		return fmt.Errorf("ensure user: %w", err)
	}
	if err := b.ensureAdditionalGroups(id, bootstrap.SandboxGroups()); err != nil {
		return fmt.Errorf("ensure additional groups: %w", err)
	}
	if wire {
		if err := b.wireConfig(); err != nil {
			return fmt.Errorf("wire config: %w", err)
		}
	}
	// Before the volumes and the sources, per seedWorkingRoot: anything wired
	// onto the working root itself carries its own ownership.
	if err := b.seedWorkingRoot(bootstrap.WorkingRoot(sandboxPaths), id); err != nil {
		return fmt.Errorf("seed working root: %w", err)
	}
	if wire {
		volumes, err := loadResolvedVolumes(id, bootstrap.Volumes)
		if err != nil {
			return fmt.Errorf("resolve volumes: %w", err)
		}
		if err := b.wireVolumes(volumes, id); err != nil {
			return err
		}
		logger.Info("wired sandbox volumes", "count", len(volumes))
	}
	// Seed the home directory after the home volume (if any) is mounted, so the
	// skeleton lands on the persistent volume rather than the image layer.
	if err := b.seedHome(id); err != nil {
		return fmt.Errorf("seed home: %w", err)
	}
	// After seedHome: it fills only an empty home from /etc/skel, so a
	// .gitconfig written before it would leave the sandbox with no skeleton.
	if err := b.seedGitConfig(id, bootstrap.Git); err != nil {
		return fmt.Errorf("seed git config: %w", err)
	}
	// After seedHome for the same reason, and before wireSources only
	// because it reads the manifest's targets rather than the trees. It
	// writes straight to its final owner: nothing chowns home afterwards.
	if !b.off(featureDirenv) {
		if err := b.seedDirenvConfig(id, bootstrap.Sources); err != nil {
			return fmt.Errorf("seed direnv config: %w", err)
		}
	}
	// After seedHome for the same reason, and written straight to their
	// final owner.
	if !b.off(featureDesktop) {
		if err := b.seedDesktopLaunchers(id, desktop.LauncherDir); err != nil {
			return fmt.Errorf("seed desktop launchers: %w", err)
		}
	}
	if !wire {
		return nil
	}
	if err := b.wireSources(bootstrap.Sources, id); err != nil {
		return err
	}
	if len(bootstrap.Sources) > 0 {
		logger.Info("wired sandbox sources", "count", len(bootstrap.Sources))
	}
	return nil
}
