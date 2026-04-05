[![CI](https://github.com/rolandh15/mattermost-plugin-filebrowser/actions/workflows/ci.yml/badge.svg)](https://github.com/rolandh15/mattermost-plugin-filebrowser/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://www.apache.org/licenses/LICENSE-2.0)
[![Mattermost Server](https://img.shields.io/badge/Mattermost-9.5%2B-00658E.svg)](https://mattermost.com)
[![Ko-fi](https://img.shields.io/badge/Ko--fi-support-FF5E5B.svg?logo=ko-fi&logoColor=white)](https://ko-fi.com/rolandhdev)

# mattermost-plugin-filebrowser

A Mattermost slash-command plugin for browsing, sharing, and uploading files on a [Filebrowser](https://github.com/filebrowser/filebrowser) instance. Built for non-technical users: every path is autocompleted live against the backend, every command is a single typed trigger, and every response is ephemeral so it never noises up a channel.

Sister project: [**krfiles**](https://github.com/rolandh15/krfiles) — the Kotlin Multiplatform Filebrowser client library. Once the cgo integration lands (v0.2.0), this plugin will call into krfiles' Kotlin/Native shared library for every Filebrowser operation, so both projects benefit from the same shared business logic.

## Commands

| Command | What it does |
|---|---|
| `/filebrowser connect` | Link your Filebrowser account. The plugin DMs you a one-time dialog asking for credentials. |
| `/filebrowser browse [path]` | List a directory as an ephemeral card with drill-down buttons. |
| `/filebrowser save <path>` | Upload the file you just attached in this channel to the given path. |
| `/filebrowser share <path>` | Create a shareable link for a file and return it to you privately. |
| `/filebrowser search <query>` | Search across your Filebrowser instance. |
| `/filebrowser disconnect` | Forget your stored credentials. |
| `/filebrowser help` | Show the full list of commands. |

All commands that accept a path support live autocomplete: as you type, the plugin queries Filebrowser and offers matching paths inline.

## Installation

1. Download the latest `mattermost-plugin-filebrowser-<version>.tar.gz` from [Releases](https://github.com/rolandh15/mattermost-plugin-filebrowser/releases).
2. In Mattermost, go to **System Console → Plugin Management → Upload Plugin** and upload the tarball.
3. Enable the plugin.
4. Open **System Console → Plugins → Filebrowser** and set the `Filebrowser URL`.
5. Each user runs `/filebrowser connect` once to link their account.

## Development

### Prerequisites

- Go 1.25+
- `jq` (for Makefile version checks)
- A local Mattermost server for end-to-end testing (optional)

### Running tests

```bash
make test        # unit tests with -race
make cover       # coverage report
make vet         # go vet
```

### Building locally

```bash
make build       # builds for host OS/arch → server/dist/plugin-<target>
make dist        # builds + bundles a host-only plugin tarball
```

Local `make dist` only bundles the current host's binary — CI produces the full linux-amd64 / linux-arm64 / darwin-amd64 / darwin-arm64 matrix on tag push.

### Architecture

```
server/
├── plugin.go              Plugin lifecycle + ExecuteCommand hook
├── configuration.go       Admin-console settings (plugin.json → struct)
├── command/
│   └── router.go          /filebrowser subcommand dispatcher
└── fb/
    ├── client.go          Filebrowser client interface
    ├── fake.go            In-memory impl used by tests
    └── <cgo/>             Real impl backed by krfiles native lib (v0.2.0)
```

Command handlers depend only on the `fb.Client` interface, so unit tests drive them with the in-memory `Fake` — no network, no Filebrowser, no flakes.

## Releasing

1. Bump `VERSION` and `plugin.json`'s `version` to the new value (both must match).
2. Commit, tag `vX.Y.Z`, and push the tag: the `release.yml` workflow validates the tag, builds all four targets, bundles them into a tarball, and attaches it to a GitHub Release automatically.

## License

Apache License 2.0 — see [LICENSE](LICENSE).
