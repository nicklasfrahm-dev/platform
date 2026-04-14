# AI Agents Guidelines

## Reasoning Protocol
Before writing any code, you MUST explicitly:
1. Restate the problem in your own words.
2. Identify edge cases and failure modes.
3. Outline your implementation approach.
4. Only then produce the code.

Never skip this — even for small tasks.

## Precision Standards
- **Explicit over Implicit**: Never assume context.
- **Conservative Interpretation**: If a type, signature, or contract is ambiguous, state the ambiguity and pick the most conservative interpretation.
- **Completeness**: Output complete, compilable code. No `// TODO` stubs unless explicitly asked.
- **Error Handling**: Prefer returning errors over panicking.

## Tool Usage
- **Read Before Edit**: Always read a file before editing it.
- **Minimal Diffs**: Change only what is needed.
- **Conceptual Verification**: Verify your edit compiles conceptually before submitting.
- **Plan Multi-file Edits**: If a task requires multiple file edits, plan them all before executing any.

## Go Specifics
- **Idiomatic Go**: Follow table-driven tests, explicit error wrapping, and interface-first design.
- **Pluggable Components**: Use the driver/registry pattern.
- **Resource Model**: Use Kubernetes-style resource model (`metadata`/`spec`/`status`) for domain objects where appropriate.

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
