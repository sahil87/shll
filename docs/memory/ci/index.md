# ci — Memory Index

Continuous-integration and release automation for shll. Per Constitution VI, releases are tag-driven (`v*`) GitHub Actions workflows; logic lives in YAML and `scripts/`, not in shll's Go code.

| Memory File | Description |
|-------------|-------------|
| [release-workflow](release-workflow.md) | `release.yml` — cross-compile, GitHub Release, Homebrew-tap update, and the help-dump → shll.ai auto-merge PR (native dump build, validate, `SHLLAI_TOKEN`, release-tag-only). |
