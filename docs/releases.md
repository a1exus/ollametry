# Releases

[← README](../README.md)

## Publishing

Before tagging, run a dry run: *Actions → Release → Run workflow* on `main`, or
`gh workflow run release.yml --ref main`. It builds the image and renders the
release notes from `[Unreleased]` into the run summary, but publishes
nothing; `notes.md` is attached to the run. A tag
then exercises the exact pipeline the dry run checked.

1. Move the `## [Unreleased]` entries in [`CHANGELOG.md`](../CHANGELOG.md)
   under a new `## [X.Y.Z] - YYYY-MM-DD` heading and add its link at the
   bottom ([Keep a Changelog](https://keepachangelog.com/en/1.1.0/)).
2. Merge that change, then tag `main`:

   ```sh
   git tag -a vX.Y.Z -m vX.Y.Z && git push origin vX.Y.Z
   ```

The release workflow then:

- builds `ghcr.io/a1exus/ollametry` for `linux/amd64` and `linux/arm64`,
  tagged `X.Y.Z`, `X.Y`, `X` (from 1.0) and `latest`, with an SBOM and build
  provenance;
- attests the image with GitHub artifact attestations;
- creates the GitHub release from that version's `CHANGELOG.md` section, with
  the image digest and how to verify it; it fails if the section is missing.

ollametry ships only as this image; there are no separate binaries.

## Verifying

```sh
gh attestation verify oci://ghcr.io/a1exus/ollametry:X.Y.Z -R a1exus/ollametry
```

## Using a released image

Running a published image with this repository's Compose setup is covered in
[install](install.md#using-a-published-image).
