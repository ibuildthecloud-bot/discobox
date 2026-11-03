# 26-11-02-023 — A machine sandbox wires its declared volumes from its own data and cache roots

- **Status**: Accepted
- **Date**: 2026-11-02; amended 2026-11-03, before anything shipped against
  it: a machine's existing content at a data path is moved into its volume
  and bound, not overlaid (§1)
- **Amends**:
  [0126](0126-a-sandbox-does-not-share-a-host-or-a-filesystem-with-its-pool.md)
  §2's "where it has none, the declared-volume mechanism does not apply at
  all"; [26-10-09-143](26-10-09-143-a-vm-sandbox-starts-its-agent-when-its-bootstrap-arrives.md)
  §1's "Declared volumes are not wired"; and
  [26-10-10-767](26-10-10-767-a-container-sandbox-boots-systemd-and-a-unit-wires-its-volumes.md)
  §2's unit conditional on a config volume "which only a pool's container
  has".
- **Relates to**: [0007](0007-declarative-sandbox-volumes-wired-by-the-sandbox-agent.md),
  [0094](0094-the-pool-cache-is-partitioned-by-the-sandbox-users-uid.md),
  [26-10-16-423](26-10-16-423-a-harness-is-a-provisioning-bundle-and-the-agent-provisions-any-box.md),
  [26-10-27-345](26-10-27-345-a-machine-sandbox-is-made-again-in-place-and-its-bootstrap-names-where-its-agent-listens.md)
  and discobox-ai/vm#23.

## Context

A harness and the platform declare volumes (ADR 0007): paths such as `%HOME%`,
`/var/lib/docker`, `~/.cache` and Claude Code's `/var/lib/claude-code`, each
`data` or `cache`, with an owner and a mode. In a Docker sandbox the pool
mounts `/.discobox/data` and `/.discobox/cache`, and `discobox-wire.service`
(26-10-10-767) binds or overlays each declared path from them before anything
uses it. That is also what *creates* each path, owned by whoever the
declaration says.

A machine sandbox (boxd, or a local VM through disco-vm) skips all of it. 0126
§2 said the declared-volume mechanism does not apply where there is no PID-1
flow, and the wire unit is conditional on a config volume no machine has. So
on a machine:

- **A declared path nothing else creates does not exist.** Claude Code's
  harness sets `CLAUDE_CODE_TMPDIR=/var/lib/claude-code` and declares it a
  `data` volume owned by the sandbox user; on a boxd sandbox `claude` fails
  with `EACCES: permission denied, mkdir '/var/lib/claude-code'`. Every path a
  harness declares outside the home has the same gap.
- **A box has two layouts.** What a sandbox keeps is spread across its disk on
  a machine and gathered under `/.discobox/data` in a container. Clearing the
  cache, a box's own skill about where things persist, the verify recipes and
  any future export each have to know which kind of box they are on.
- **A machine's durable tree cannot be separated from its disk.** 26-10-27-345
  rebuilds a machine in place because its data *is* its disk; a data volume of
  its own (discobox-ai/vm#23) would need the data to already live in one place
  to be moved onto it.

## Decision

### 1. A machine has `/.discobox/data` and `/.discobox/cache`

A Linux machine sandbox has both roots, as directories on its own disk. Every
declared volume is wired from them exactly as in a container: the same
`volumeDir` layout (data per target, cache partitioned by uid under
`.users/<uid>`, a `scope: shared` path at the cache root), the owner and mode
the declaration names, parents before children.

Every path is a bind on a machine, never an overlay. A data path the machine
already has content at, the first time it is wired, has that content moved
into its volume directory and is then bound: the disk is the box's own, not
an image shared with other boxes, so there is no lower layer to keep. Each
entry is moved by itself, so a boot cut short leaves the rest to move at the
next one, and the layout is recorded only once all of it has moved. A
container keeps ADR 0107's rule: an overlay over what its image ships.

The cache stays private to the box (0126 §2 stands): nothing outside the
machine shares it. Its uid partition is kept anyway, so the layout is one
layout and a machine's cache can later move to a shared volume without a
re-seed. Source-scoped pool data (`/.discobox/data-per-source`) stays
unavailable on a machine, as 0126 §2 says.

When disco-vm gives an instance a volume (discobox-ai/vm#23), the roots are
where it mounts. Nothing else changes: the wiring reads directories, not
devices.

### 2. The same unit wires both, before `sysinit.target`, on every boot

`discobox-wire.service` runs on a machine too. Its condition becomes "this box
has a sandbox": the config volume's bootstrap in a container, the placed
bootstrap at `/etc/discobox/sandbox.json` on a machine. It keeps its ordering
(before `local-fs.target`, `sysinit.target` and tmpfiles), so nothing that
uses a wired path — the provider's own Docker on boxd among it — starts ahead
of the wiring, and it runs on every boot, since a machine's mounts do not
survive a restart either.

What a machine has no volume for is not wired: its config stays the placed
bootstrap, its secrets and sources keep arriving through the intake
(26-10-08-127). The unit wires the sandbox user and the declared volumes; on a
container it also binds config, secrets and sources, as today.

A wiring failure must not let the box run, on either kind: in a container the
manager exits (26-10-10-767 §5); on a machine it powers off, which the pool
reports as a stopped sandbox, the way it reports an exited container.

### 3. A machine's first placement restarts it before anything runs

A machine boots before its bootstrap is placed, so its own services — boxd's
dockerd and containerd — are already running on the paths the wiring would
mount over. The first placement therefore places the bootstrap and the agent
layer, starts nothing, and restarts the machine; the wire unit then runs at
its first real boot, before those services, and the agent and feature
provisioning start after it. Placement already restarts a machine whose
placed bootstrap moved (26-10-27-345); a first placement becomes one more
reason to.

A machine provisioned before this decision is wired at its next start. Its
existing content at a declared data path — the home, `/var/lib/docker` — is
moved into `/.discobox/data` and bound back (§1), so nothing it held is
hidden.

## Alternatives rejected

- **Overlay a machine's existing content, as a container overlays what its
  image ships.** The first rule written here. Rejected because a boxd
  machine's `/var/lib/docker` and `/var/lib/containerd` always hold content
  by then — its own dockerd ran on the boot before the placement — and
  Docker's `overlay2` driver and containerd's overlayfs snapshotter refuse a
  data root that is itself on overlayfs: Docker in the sandbox would stop
  working. A bind over the content instead would hide it, and with it an
  existing machine's home.

- **Only create each declared path, owned as declared, without mounting.**
  The smallest fix for `/var/lib/claude-code`. Rejected because it keeps two
  layouts: the gap it closes is one symptom of a machine's durable tree having
  no single place, and every consumer of that tree (cache clearing, export,
  a data volume) would still need a machine branch.
- **Wire on a live machine, stopping and restarting the units that use the
  wired paths.** Avoids the first-boot restart. Rejected because the pool
  cannot know every unit of a provider's image that holds a declared path
  open, and a mount laid under a running process splits its view of the path
  silently rather than failing.
- **Have each feature create its paths in its own provisioning steps.**
  A declared volume's owner is the sandbox user's uid, which a snapshot shared
  by every box cannot bake in, and it would duplicate in each feature what the
  declaration already says.

## Deferred

- **Exporting and importing a machine sandbox's tree.** Still refused
  (`ErrUnsupported`). This decision makes `/.discobox/data` the tree to
  export, as in a container; revisit when a machine pool needs export.
- **A machine's data on a volume of its own**, and with it rebuilding a
  machine rather than reusing its disk (26-10-27-345): revisit when disco-vm
  gives an instance a volume (discobox-ai/vm#23).

## Consequences

- `boot.Provision` no longer skips volumes on a machine; the wiring moves to
  the unit on both kinds, and `Provision`'s once-per-sandbox record keeps the
  account and seeding steps it alone does.
- `discobox-wire.service`'s condition and failure action differ by box kind,
  and the agent layer the pool places on a machine already carries the unit.
- The machine runtime's first placement restarts the machine before the agent
  starts, adding one guest boot to a machine sandbox's create.
- The sandbox's `discobox` skill and the verify recipes describe one layout.
- A machine's first wiring of a populated path moves it within the disk:
  a rename per entry on the one filesystem, a copy only where the path is on
  another.
