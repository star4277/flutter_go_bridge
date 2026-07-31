# Releasing

Releases are created manually from GitHub Actions. The workflow builds the
`flutter_go_bridge_codegen` CLI with the Makefile matrix and uploads one archive per supported
platform and architecture.

## Start a release

Open **Actions → Release → Run workflow** and provide:

- `version`: the base semantic version, such as `v1.2.3` or `1.2.3`;
- `pre_release`: whether to publish the next beta release.

The workflow normalizes a missing `v` prefix. A stable release uses the input directly:

```text
input: v1.2.3
tag:   v1.2.3
```

For a pre-release, the workflow queries existing GitHub Releases and increments the beta suffix:

```text
no v1.2.3-beta* release exists  -> v1.2.3-beta1
v1.2.3-beta1 already exists     -> v1.2.3-beta2
v1.2.3-beta2 already exists     -> v1.2.3-beta3
```

The workflow is serialized with GitHub Actions concurrency so two manual runs cannot calculate the
same beta number at the same time.

## Replacing an existing release

The final release tag is the replacement key. If that tag already has a GitHub Release, the workflow:

1. deletes the existing Release and its tag;
2. builds fresh archives with the current source revision;
3. creates the Release again with regenerated assets and release notes.

If only a tag exists without a Release, the tag is deleted before the replacement Release is created.
This means rerunning `v1.2.3` intentionally replaces the previous `v1.2.3` contents.

## Version propagation

The workflow exports the resolved final value through:

```text
FLUTTER_GO_BRIDGE_VERSION
```

The same variable is used by the Makefile and the CLI build:

```sh
FLUTTER_GO_BRIDGE_VERSION=v1.2.3 make linux-amd64
```

The Makefile uses it in the archive name and injects it into `main.version` with Go `-ldflags`.
`flutter_go_bridge_codegen -v` and `--version` then report both the release version embedded in that
binary and the Go toolchain used to build it:

```text
flutter_go_bridge_codegen version v1.2.3
Build with go1.25.0
```

The release workflow verifies both lines before uploading artifacts.

At runtime, an explicitly set `FLUTTER_GO_BRIDGE_VERSION` overrides the embedded value. If neither
the environment nor build flags provide a value, the CLI uses:

```text
v0.0.1-snapshot
```

This default is for local development only. The release workflow always supplies an explicit version.

## Build boundary

This release workflow publishes the `flutter_go_bridge_codegen` command. Its Makefile `ldflags` do not
configure the Gokit build of an application's Go cgo library. Gokit remains responsible for the
application's native-library build flags.

The release matrix currently includes Windows, Linux, and macOS targets. Archives are uploaded as
GitHub Release assets with names based on command, architecture, operating system, and version.
