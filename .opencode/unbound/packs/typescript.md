---
pack_id: typescript
language: TypeScript
version: 1.0.0
---
<!-- scaffolded by uf vdev -->
<!-- scaffolded by uf vdev -->
<!-- scaffolded by uf vv0.6.1 -->

# Convention Pack: TypeScript

This convention pack defines TypeScript-specific review
criteria for The Divisor PR reviewer framework. Persona
agents load this pack dynamically at review time to
evaluate TypeScript codebases against language-specific
coding style, architectural patterns, security checks,
testing conventions, and documentation requirements.

Rules use RFC 2119 severity indicators: [MUST] for
mandatory requirements, [SHOULD] for strong recommendations,
and [MAY] for optional best practices.

---

## Coding Style

- **CS-001** [MUST] Code MUST pass ESLint with the
  project's configured ruleset. No lint errors or
  warnings committed without explicit disable comments
  that include a justification.

- **CS-002** [MUST] Code MUST be formatted with Prettier
  (or the project's configured formatter). Formatting
  MUST NOT be mixed with logic changes in the same
  commit.

- **CS-003** [MUST] The `any` type MUST NOT be used.
  Use `unknown` for truly unknown types, then narrow
  with type guards. Existing `any` usages SHOULD be
  eliminated incrementally; new `any` introductions
  are always rejected.

- **CS-004** [MUST] Strict null checks MUST be enabled
  (`strictNullChecks: true` in tsconfig.json). Code
  MUST handle `null` and `undefined` explicitly —
  no reliance on loose truthiness checks for nullable
  values.

- **CS-005** [MUST] All exported functions, classes,
  interfaces, and type aliases MUST have explicit type
  annotations on their signatures. Return types MUST
  be annotated on exported functions — do not rely on
  type inference for public API surfaces.

- **CS-006** [MUST] Naming MUST follow TypeScript
  conventions: `camelCase` for variables, functions,
  and methods; `PascalCase` for classes, interfaces,
  type aliases, and enums; `UPPER_SNAKE_CASE` for
  constants. Interface names MUST NOT use the `I`
  prefix (e.g., use `UserService` not `IUserService`).

- **CS-007** [MUST] Imports MUST be organized in groups
  separated by blank lines: (1) Node.js built-ins,
  (2) external packages, (3) internal/project modules,
  (4) relative imports. Within each group, imports
  SHOULD be sorted alphabetically.

- **CS-008** [MUST] Use `const` by default. Use `let`
  only when reassignment is required. `var` MUST NOT
  be used under any circumstances.

- **CS-009** [SHOULD] Complex functions (more than one
  branch or side effect) SHOULD have explicit return
  type annotations even when not exported. Arrow
  functions used as simple expressions MAY rely on
  inference.

- **CS-010** [SHOULD] Prefer destructuring for object
  and array access where it improves readability.
  Prefer template literals over string concatenation
  for interpolated strings.

---

## Architectural Patterns

- **AP-001** [SHOULD] Modules SHOULD use barrel exports
  (`index.ts`) to define their public API. Internal
  implementation files SHOULD NOT be imported directly
  by consumers outside the module boundary.

- **AP-002** [SHOULD] Services and repositories SHOULD
  accept dependencies through constructor injection or
  factory function parameters — not through module-level
  singletons or global state. This enables testing and
  composability.

- **AP-003** [SHOULD] Business logic SHOULD follow the
  repository/service pattern: repositories handle data
  access, services handle business rules. Controllers
  or handlers SHOULD be thin — delegating to services.

- **AP-004** [MUST] Data structures SHOULD prefer
  immutability. Use `readonly` properties, `ReadonlyArray`,
  and `Readonly<T>` utility types where appropriate.
  Avoid mutating function arguments. MUST NOT mutate
  shared state without explicit synchronization.

- **AP-005** [MUST] All Promises MUST be handled. No
  fire-and-forget `async` calls. Use `await` for
  sequential async operations. Prefer `Promise.all()`
  or `Promise.allSettled()` for concurrent operations.
  The `@typescript-eslint/no-floating-promises` rule
  MUST be enabled.

- **AP-006** [SHOULD] Module boundaries SHOULD be
  enforced through explicit exports. Circular
  dependencies MUST be avoided — use dependency
  inversion or extract shared types into a common
  module to break cycles.

---

## Security Checks

- **SC-001** [MUST] `eval()`, `new Function()`, and
  `setTimeout`/`setInterval` with string arguments
  MUST NOT be used. These introduce code injection
  vulnerabilities.

- **SC-002** [MUST] All user inputs MUST be validated
  and sanitized before use. Use schema validation
  libraries (Zod, Joi, class-validator) for structured
  input. Never trust client-supplied data for database
  queries, file paths, or command execution.

- **SC-003** [MUST] Secrets, API keys, tokens, and
  credentials MUST NOT be hardcoded in source files.
  Use environment variables or a secrets manager.
  Files matching common secret patterns (`.env`,
  `credentials.json`, `*.pem`, `*.key`) MUST NOT
  be committed.

- **SC-004** [SHOULD] Web applications SHOULD set
  Content Security Policy (CSP) headers. Inline
  scripts and styles SHOULD be avoided in favor of
  external files with nonce-based CSP.

- **SC-005** [MUST] User-generated content rendered
  in HTML MUST be escaped or sanitized to prevent
  XSS attacks. Use framework-provided escaping (React
  JSX auto-escaping, Angular sanitization) or a
  dedicated library like DOMPurify. Never use
  `innerHTML` or `dangerouslySetInnerHTML` with
  unsanitized input.

- **SC-006** [SHOULD] Dependencies SHOULD be audited
  regularly with `npm audit` or equivalent tooling.
  PRs that introduce new dependencies SHOULD note the
  package's maintenance status, license, and known
  vulnerabilities. Dependencies with critical CVEs
  MUST NOT be merged.

---

## Testing Conventions

- **TC-001** [MUST] All new code MUST include tests.
  New functions, classes, and modules MUST have
  corresponding unit tests. Bug fixes MUST include
  a regression test that fails without the fix.

- **TC-002** [MUST] External dependencies (APIs,
  databases, file system, network) MUST be mocked
  or stubbed in unit tests. Tests MUST NOT rely on
  external services or network connectivity.

- **TC-003** [MUST] Error paths and edge cases MUST
  be tested explicitly. Tests SHOULD cover: null/
  undefined inputs, empty arrays/objects, boundary
  values, thrown exceptions, and rejected Promises.

- **TC-004** [SHOULD] Snapshot tests SHOULD be used
  for UI component output or complex serialized
  structures where manual assertion is impractical.
  Snapshot updates MUST be reviewed — do not blindly
  accept snapshot changes. Snapshots SHOULD NOT be
  used as a substitute for behavioral assertions.

- **TC-005** [MUST] Tests MUST be independent — no
  shared mutable state between tests, no reliance
  on test execution order. Each test MUST set up
  its own fixtures and tear them down.

- **TC-006** [MUST] Async tests MUST properly await
  all asynchronous operations. Use `async`/`await`
  in test functions. Ensure the test framework is
  configured to detect unhandled Promise rejections.
  Never return a Promise without awaiting it in the
  test body.

- **TC-007** [SHOULD] Test descriptions SHOULD be
  meaningful and describe the expected behavior, not
  the implementation. Use the pattern: "should [verb]
  when [condition]" (e.g., `"should return 404 when
  user not found"`).

- **TC-008** [SHOULD] Test files SHOULD be co-located
  with their source files (e.g., `user.service.ts`
  and `user.service.test.ts` in the same directory)
  or placed in a `__tests__/` directory at the same
  level. Test file naming MUST follow the project's
  established convention consistently.

---

## Documentation Requirements

- **DR-001** [MUST] All exported functions, classes,
  interfaces, and type aliases MUST have JSDoc
  comments. JSDoc MUST include a description, `@param`
  tags for each parameter, and `@returns` for the
  return value. Complex types SHOULD include `@example`
  blocks.

- **DR-002** [SHOULD] README.md SHOULD be updated when
  new features, configuration options, or public APIs
  are added. Installation, usage, and configuration
  sections SHOULD reflect the current state of the
  project.

- **DR-003** [SHOULD] A changelog (CHANGELOG.md or
  equivalent) SHOULD be maintained. Entries SHOULD
  follow Keep a Changelog format. Breaking changes
  MUST be clearly marked.

- **DR-004** [SHOULD] Public API documentation SHOULD
  be generated from JSDoc comments using a tool like
  TypeDoc. API documentation SHOULD be kept in sync
  with the source — stale API docs are worse than no
  docs.

---

## Custom Rules

<!-- This section is intentionally empty in the canonical pack. Project-specific custom rules belong in typescript-custom.md -->
