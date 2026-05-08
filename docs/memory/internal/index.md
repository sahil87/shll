# internal — Memory Index

Shared internal packages used by `cmd/shll/`. Per Constitution I (Security First), every subprocess invocation in shll routes through `internal/proc`; no other package may import `os/exec`.

| Memory File | Description |
|-------------|-------------|
| [proc](proc.md) | Centralized subprocess wrapper — `Run` (capture), `RunForeground` (inherited stdio), `ErrNotFound` sentinel, `Runner` test seam. |
