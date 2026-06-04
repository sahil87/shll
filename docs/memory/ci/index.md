# ci — Memory Index

Continuous-integration and release automation for shll. Per Constitution VI, releases are tag-driven (`v*`) GitHub Actions workflows; logic lives in YAML and `scripts/`, not in shll's Go code.

| Memory File | Description |
|-------------|-------------|
| [release-workflow](release-workflow.md) | `release.yml` — cross-compile, publish a GitHub Release, and update the Homebrew tap. No longer pushes to shll.ai (help-push transport torn down in change 7huv; shll.ai now pulls via `shll help-dump`). |
