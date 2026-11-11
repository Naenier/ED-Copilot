# Build and delivery

These developer scripts automate checks, packaging, release verification and
installation. They do not add developer controls to the application interface.
PowerShell 7.2+, Go and Node.js are required for a local build. Delivery of an
already published Windows release needs only PowerShell 7.2+.

## One local build command

```powershell
./build.ps1 -Smoke -Package -Installer
```

This runs frontend and delivery contract tests, Go tests and vet, builds the EXE,
runs isolated headless/native/stress scenarios, packages the portable ZIP and
Setup, tests the installer and writes checksums. `app.Version` is the single
version source; Windows resources are generated from it. The checked-in resource
JSON is a template. `-SkipTests` is for investigation and produces a manifest
that cannot pass release validation.

## Publish source, then deliver automatically

Set `app.Version` and add `docs/RELEASE-{version}.md`. Push the source commit with
`[release]` in its commit message. A normal authenticated Git checkout can use
its existing credential helper. No credentials are stored in these scripts.

When a GitHub connector publishes source instead of Git, prepare its deterministic
allowlisted upload with:

```powershell
./scripts/prepare-source.ps1 -PreviousManifest <previous-publication.json> -OutputDirectory <staging-directory>
```

The output contains changed Git blobs, deletions, the expected parent commit and
base tree, and four-aligned base64 chunks. The connector creates the commit and
updates `main` with a compare-and-swap check. Connector authentication stays in
the connector; a standalone shell cannot borrow it.

Once the source commit has been pushed:

```powershell
./scripts/deliver.ps1 -Release -Install
```

The command takes the exact commit from the checked-out Git HEAD. In a connector
workspace without `.git`, pass `-PublicationManifest <published-manifest.json>`;
the connector's publication record must contain the resulting `commit`. An
explicit `-ExpectedCommit <full-sha>` is also supported. The command waits for
that exact source revision, not a previous `latest`
release. GitHub runs the race detector and the full build/check command once.
The release job downloads the already verified workflow artifact and publishes
the Setup, portable ZIP, checksums and release manifest without rebuilding.
The four smoke reports stay in the workflow artifact. A newer `main` commit
stops an obsolete release. Version, tag, asset names and release notes are derived
automatically.

Delivery then checks the successful workflow run, tag-to-commit mapping, release
manifest, GitHub SHA256 digests, checksum file and ZIP executable. The workflow
validates all smoke reports before it publishes the release.
Repeated runs reuse verified downloads. Installation checks the final EXE hash
and version, uninstall registration, critical profile preservation and an isolated
headless startup. One JSON report records the final result; manual checksum
commands and repeated installer checks are unnecessary.

## Install the latest stable release

```powershell
./scripts/deliver.ps1 -Install
```

This resolves the stable release's exact commit and performs the same checks.
Public downloads do not require a GitHub token. The registered installation
directory is used automatically; `-InstallDirectory` can select a directory for
a first installation. Existing installations cannot be silently redirected.

The running application is never killed. Files are staged while it is open;
installation waits for full exit, including the tray process. If it remains open,
the report says `deferred`, and a later invocation resumes from the verified
cache. Closing to the tray does not constitute full exit.

If version and executable hash already match, delivery is a successful no-op.
A different executable under the same version requires the explicit developer
option `-AllowSameVersionRepair`; source and artifact verification still apply.
Downgrades are refused. Published versions are immutable: changed source needs
a new version.

Backups, verified packages and `delivery-last.json` are stored under
`%LOCALAPPDATA%\ED Copilot Delivery`. Critical commander files are backed up and
hashed after clean exit. Volatile WebView2 caches and updater bookkeeping are
excluded from this checkpoint; credentials are neither decrypted nor logged.
The script does not force a reboot or launch the normal game-connected session.

Source tree comparison normalizes CRLF to LF in UTF-8 text to match Git checkout
rules. Uploaded Git blob hashes and every downloadable artifact remain checked
against their exact bytes. `release-manifest.json` records the digest format.

## Tests

```powershell
./scripts/delivery-tests.ps1
```

Contract tests use mocked downloads and temporary synthetic packages/profiles.
They cover provenance, tampering, ZIP contents, source hashing, cache reuse,
same-version behavior, downgrades, URL policy and profile preservation. They make
no network requests and never install into a real application directory.
