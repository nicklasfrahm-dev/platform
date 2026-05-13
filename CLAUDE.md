## Pull Requests

- **NEVER include a test plan section** in PR descriptions.
- **NEVER include attribution** to any AI assistant (Claude, Cursor, Copilot, etc.).
- A PR description must contain only a concise summary of the changes — nothing else.

## Git

All commits and PR titles must follow the **Conventional Commits** specification: `<type>(<scope>): <description>`

- **Types**: `feat`, `fix`, `chore`
- **Scopes**: `charts`, `ci`, `config`, `deps`, `docs`, `llm`, `tools` — or any directory under `cmd/` or package under `pkg/`
- **Description**: imperative mood, lowercase

See `AGENTS.md` for full agent guidelines.
