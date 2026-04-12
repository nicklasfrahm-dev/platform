# AI Agents Guidelines

---

## git

All commits must follow the **Conventional Commits** specification.

### Format
`<type>(<scope>): <description>`

### Rules
- **Types**:
  - `feat`: A new feature.
  - `fix`: A bug fix.
- **Scope**: The scope (e.g., `llm`, `ci`, `argocd`) is **always required**.
- **Description**: The message must be written in the **imperative mood** (e.g., "add feature" instead of "added feature" or "adds feature").

### Examples
- `feat(llm): add gemma4 deployment config`
- `fix(argocd): resolve authentication issue`
