# Release Tags

- Releases use semantic version tags with the `v` prefix.
- Every release increments the patch version from the highest existing release tag unless a minor or major release is explicitly requested.
- The npm package version, package-lock root version, Git tag, and release announcement must use the same version.
- Before pushing a release, run `go test ./...`, `npm run test:node`, and `git diff --check`.
