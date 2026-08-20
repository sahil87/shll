---
type: memory
description: "Removal history for memory topic files — tombstone rows relocated out of generated indexes, each citing the change that removed the file and where its content went."
---
# Removed memory files

Tombstone rows lifted verbatim from generated indexes when their topic files were deleted. The links are dead by definition — these rows are the removal record.

## cli

- [agent-setup](agent-setup.md) (from `docs/memory/cli/index.md`) — `shll agent-setup` — places ONE thin `shll-toolkit` Agent Skill at `~/.agents/skills/` + `~/.claude/skills/`, then delegates run-kit's dashboard hooks to `run-kit agent setup` (min rk v3.16.23; `--yes`/`-y` forwards `--yes` to that delegation for unattended runs). Idempotent (write/overwrite/delete, no sentinel); `--print`/`--uninstall` modes. SKILL.md is a Go constant; `agentSkillDescription()` builds the frontmatter from the Roster: `SkillHint` clauses plus each `ProactiveHint`. — content consolidated into [cli/setup](/cli/setup.md) (260819-7p6b-consolidate-setup-command)
- [shell-setup](shell-setup.md) (from `docs/memory/cli/index.md`) — `shll shell-setup [shell]` (alias `shell-install`) — sentinel-wrapped rc-file block, pure rc-wiring (eval line only), idempotent install/`--print`/`--uninstall`, stale-export migration. — content consolidated into [cli/setup](/cli/setup.md) (260819-7p6b-consolidate-setup-command)
