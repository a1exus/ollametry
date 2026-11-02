# Releases

[← README](../README.md)

## Publishing

Before tagging, run a dry run: *Actions → Release → Run workflow* on `main`, or
`gh workflow run release.yml --ref main`. It builds the image and the archives
and renders the release notes from `[Unreleased]` into the run summary, but
publishes nothing; the archives and `notes.md` are attached to the run. A tag
then exercises the exact pipeline the dry run checked.

1. Move the `## [Unreleased]` entries in [`CHANGELOG.md`](../CHANGELOG.md)
   under a new `## [X.Y.Z] - YYYY-MM-DD` heading and add its link at the
   bottom ([Keep a Changelog](https://keepachangelog.com/en/1.1.0/)).
2. Merge that change, then tag `main`:

   ```sh
   git tag -a vX.Y.Z -m vX.Y.Z && git push origin vX.Y.Z
   ```

The release workflow then:

- builds reproducible binary archives for macOS and Linux (`arm64`,
  `amd64`) with `make dist`, the same pinned Go image and flags as the
  container image, and attests them;
- builds `ghcr.io/a1exus/ollametry` for `linux/amd64` and `linux/arm64`,
  tagged `X.Y.Z`, `X.Y`, `X` (from 1.0) and `latest`, with an SBOM and build
  provenance;
- attests the image with GitHub artifact attestations;
- creates the GitHub release from that version's `CHANGELOG.md` section,
  attaching the archives and `SHA256SUMS`; it fails if the section is missing.

## Verifying

```sh
gh attestation verify oci://ghcr.io/a1exus/ollametry:X.Y.Z -R a1exus/ollametry
gh attestation verify ollametry_X.Y.Z_darwin_arm64.tar.gz -R a1exus/ollametry
```

To check reproducibility, run `make dist VERSION=vX.Y.Z` on the tagged commit
and compare `dist/SHA256SUMS` with the release's. Installing is covered in
[install](install.md).

## Using a released image

To run a published image instead of building one, add a
`compose.override.yaml` next to `compose.yaml` (Compose reads it
automatically; `!reset` needs Compose 2.24 or newer):

```yaml
services:
  ollametry:
    build: !reset null
    image: ghcr.io/a1exus/ollametry:X.Y.Z
    pull_policy: missing
```

`make up` then pulls the image instead of building it.
