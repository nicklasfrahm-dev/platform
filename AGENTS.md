# AI Agents Guidelines

## MANDATORY PLANNING PHASE

Before taking ANY action on a task, you MUST explicitly:

1. Restate the problem in your own words.
2. Identify edge cases and failure modes.
3. Output a TODO list in this exact format:

TODO:

- [ ] <concrete, atomic step>
- [ ] <concrete, atomic step>
- [ ] <concrete, atomic step>
      ...

No tool calls are permitted until the TODO list is written.

Rules for TODOs:

- Each item must be a single, verifiable action (read file X, run command Y, edit function Z)
- No vague steps like "figure out the problem" — be specific about what you will look at and why
- Include an explicit verification step at the end (run tests, confirm output, check diff)
- You may revise the TODO list mid-task if you discover new information, but you must re-emit the full updated list before continuing

Only after writing the TODO list should you begin executing steps, checking them off as you go:

- [x] <completed step>

---

## TOOL CALLING RULES

1. **One tool call at a time.** Never batch multiple tool calls in a single turn. Wait for the result before proceeding.

2. **Ground every tool call in your TODO.** Before each tool call, write one sentence explaining which TODO item it serves.

3. **Never guess at file paths or symbol names.** If you are uncertain, use search or list tools first to verify.

4. **Never fabricate tool results.** If a tool returns an error or empty output, report it honestly and adjust your plan.

5. **Read before write.** Always read or inspect a file before editing it, unless creating a brand new file from scratch.

6. **After every tool result, think before acting.** Write a brief observation (1–3 sentences) about what the result tells you, then decide the next step.

7. **Minimal diffs.** Change only what is needed.

8. **Conceptual verification.** Verify your edit compiles conceptually before submitting.

If a parameter value is unclear, stop and ask the user rather than guessing.

---

## REASONING STYLE

Think step by step. When facing ambiguity:

- State what you know and what you don't know
- Identify the smallest action that reduces uncertainty
- Do not proceed with an edit until you are confident in what the code does

When you encounter an error:

- Quote the exact error message
- Hypothesize the cause before attempting a fix
- Verify your fix actually resolves the root cause, not just the symptom

---

## PRECISION STANDARDS

- **Explicit over Implicit**: Never assume context.
- **Conservative Interpretation**: If a type, signature, or contract is ambiguous, state the ambiguity and pick the most conservative interpretation.
- **Completeness**: Output complete, compilable code. No `// TODO` stubs unless explicitly asked.
- **Error Handling**: Prefer returning errors over panicking.

---

## OUTPUT FORMAT

Structure your responses as follows:

1. **TODO** (mandatory, before any tools)
2. **Step execution** — for each step: intent sentence → tool call → observation
3. **Summary** — what was done, what changed, what the user should know

Keep prose concise. Prefer code and concrete facts over explanation. Do not apologize or hedge excessively.

---

## TOOL CALL FORMAT

When calling a tool, emit it as a clean JSON block with no extra prose inside the block:

```json
{
  "name": "<tool_name>",
  "parameters": {
    "<key>": "<value>"
  }
}
```

---

## GO SPECIFICS

- **Idiomatic Go**: Follow table-driven tests, explicit error wrapping, and interface-first design.
- **Pluggable Components**: Use the driver/registry pattern.
- **Resource Model**: Use Kubernetes-style resource model (`metadata`/`spec`/`status`) for domain objects where appropriate.

---

## GIT

All commits must follow the **Conventional Commits** specification.

### Format

`<type>(<scope>): <description>`

### Rules

- **Types**: `feat` (new feature), `fix` (bug fix)
- **Scope**: Always required (e.g., `llm`, `ci`, `argocd`)
- **Description**: Written in imperative mood ("add feature", not "added" or "adds")
- **PR Description**: Never include a test plan section or attribution to any AI assistant (Crush, OpenCode, Claude, etc.)

### Examples

- `feat(llm): add gemma4 deployment config`
- `fix(argocd): resolve authentication issue`
