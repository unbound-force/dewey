---
pack_id: python
language: Python
version: 1.0.0
---
<!-- scaffolded by uf vdev -->

# Convention Pack: Python

This convention pack defines Python-specific review
criteria for The Divisor PR reviewer framework. Persona
agents load this pack dynamically at review time to
evaluate Python codebases against language-specific
coding style, architectural patterns, security checks,
testing conventions, type annotations, and documentation
requirements.

Rules use RFC 2119 severity indicators: [MUST] for
mandatory requirements, [SHOULD] for strong recommendations,
and [MAY] for optional best practices.

---

## Coding Style

- **CS-001** [MUST] Code MUST be auto-formatted with a
  consistent formatter (`black`, `ruff format`, or
  equivalent). No manual formatting overrides. Line
  length MUST match the project's configured limit.
  Formatting changes MUST NOT be mixed with logic
  changes in the same commit.

- **CS-002** [MUST] Imports MUST be auto-sorted with a
  consistent tool (`isort`, `ruff` with isort rules, or
  equivalent). Organize in groups separated by blank
  lines: standard library, third-party packages,
  local/project modules.

- **CS-003** [MUST] Code MUST pass a linter (`flake8`,
  `ruff check`, or equivalent) with the project's
  configured ruleset. No lint errors committed without
  explicit suppression comments that include the specific
  code and justification (e.g., `# noqa: E501 - URL`).

- **CS-004** [MUST] Use `snake_case` for functions,
  methods, variables, and module names. Use `PascalCase`
  for class names. Use `UPPER_SNAKE_CASE` for module-level
  constants.

- **CS-005** [MUST] All functions that can fail MUST raise
  specific exception types or return typed results. Never
  silently swallow exceptions. Every `except` clause MUST
  either handle, re-raise, or log the exception with
  context.

- **CS-006** [MUST] Bare `except:` and `except Exception:`
  MUST NOT be used unless re-raising or logging with full
  context. Catch the most specific exception type possible.

- **CS-007** [MUST] Use f-strings for string formatting.
  Do not use `%` formatting or `.format()` in new code.

- **CS-008** [MUST] No mutable default arguments. Use
  `None` as default and initialize inside the function
  body (e.g., `def foo(items=None): items = items or []`).

- **CS-009** [MUST] Use `with` statements for all resource
  management (files, database connections, locks, network
  sessions). Never rely on garbage collection for cleanup.

- **CS-010** [SHOULD] Keep functions focused on a single
  responsibility. Functions exceeding ~50 lines of logic
  SHOULD be evaluated for decomposition.

- **CS-011** [SHOULD] Use `pathlib.Path` for filesystem
  path construction instead of `os.path.join` in new code.

- **CS-012** [SHOULD] Use list/dict/set comprehensions
  where they improve readability. Avoid nested
  comprehensions deeper than two levels.

- **CS-013** [SHOULD] Prefer `dataclasses`, Pydantic
  models, or typed NamedTuples over plain dicts for
  structured data with known schemas.

---

## Architectural Patterns

- **AP-001** [MUST] Each module MUST have a single,
  well-defined responsibility. A module that handles both
  business logic and I/O coordination is a violation.

- **AP-002** [SHOULD] Dependencies SHOULD be injected
  rather than hard-instantiated. Functions and constructors
  SHOULD accept interfaces or abstractions rather than
  concrete implementations where testability benefits.

- **AP-003** [MUST] Circular imports MUST NOT exist. If
  module A imports module B, module B MUST NOT import
  module A (directly or transitively). Use local imports,
  `TYPE_CHECKING` guards, or extract shared types to break
  cycles.

- **AP-004** [SHOULD] Configuration SHOULD be loaded from
  environment variables, config files, or framework
  settings (e.g., `django.conf.settings`), not hardcoded.

- **AP-005** [SHOULD] Long modules (>500 lines) SHOULD be
  evaluated for splitting into submodules with a package
  `__init__.py` that re-exports the public API.

- **AP-006** [MUST] Application code MUST NOT import from
  test modules. Test utilities shared across test files
  SHOULD live in `conftest.py` or a dedicated test helpers
  package.

---

## Security Checks

- **SC-001** [MUST] Never hardcode secrets, API keys,
  tokens, or credentials in source code. Secrets MUST be
  loaded from environment variables, secret managers, or
  encrypted configuration. Files matching common secret
  patterns (`.env`, `credentials.json`, `*.pem`, `*.key`)
  MUST NOT be committed.

- **SC-002** [MUST] All user input MUST be validated and
  sanitized before use. Use parameterized queries for
  database access -- never string concatenation or
  f-string interpolation in SQL.

- **SC-003** [MUST] Use `defusedxml` or equivalent for
  XML parsing of untrusted input. Never use
  `xml.etree.ElementTree` or `lxml` with untrusted data
  without disabling entity expansion. Test-only XML
  parsing of trusted fixtures MAY use standard library
  parsers.

- **SC-004** [MUST] Code MUST pass security static
  analysis (`bandit`, `ruff` S rules, or equivalent).
  Address all HIGH and CRITICAL findings before merge.

- **SC-005** [SHOULD] Dependencies SHOULD be audited
  regularly with `pip-audit`, Dependabot, or equivalent
  tooling. PRs introducing new dependencies SHOULD note
  maintenance status and known vulnerabilities.
  Dependencies with critical CVEs MUST NOT be merged.

- **SC-006** [MUST] File operations with user-supplied
  paths MUST validate against directory traversal. Use
  `os.path.realpath()` or `pathlib.Path.resolve()` and
  verify the result is within the expected root.

- **SC-007** [MUST] Never pass user-supplied or external
  input to `shell=True` subprocess calls. [SHOULD] Prefer
  `subprocess.run()` with list arguments and `shell=False`
  for new code. Internal utility wrappers that use
  `shell=True` with hardcoded or developer-controlled
  commands are acceptable when documented.

- **SC-008** [SHOULD] Set safe file permissions when
  creating files: `0o644` for regular files, `0o755` for
  executable scripts and directories. Avoid world-writable
  permissions.

---

## Testing Conventions

- **TC-001** [MUST] Use `pytest` as the test runner. Do
  not use `unittest.TestCase` for new tests. Existing
  `TestCase` subclasses are acceptable until migrated.

- **TC-002** [MUST] New functionality MUST be accompanied
  by tests covering the primary success path and at least
  one failure/edge case path.

- **TC-003** [MUST] Bug fixes MUST include a regression
  test that reproduces the original failure and verifies
  the fix.

- **TC-004** [MUST] External dependencies (APIs, databases,
  filesystem, network) MUST be mocked in unit tests using
  `unittest.mock.patch`, `pytest-mock`, or `pytest`
  fixtures. Tests MUST NOT require external services or
  network connectivity.

- **TC-005** [MUST] Tests MUST be isolated -- each test
  MUST be independently runnable without depending on
  execution order or shared mutable state.

- **TC-006** [SHOULD] Use `pytest.mark.parametrize` for
  table-driven tests when exercising multiple input/output
  combinations for the same function.

- **TC-007** [SHOULD] Test names SHOULD clearly describe
  the scenario: `test_<function>_<condition>_<expected>`
  (e.g., `test_parse_empty_input_returns_none`).

- **TC-008** [SHOULD] Use `factory-boy`, `pytest` fixtures,
  or equivalent for test data construction. Avoid large
  inline dicts or manual object construction repeated
  across tests.

- **TC-009** [SHOULD] Coverage SHOULD be measured with
  `pytest-cov` or equivalent. A `--cov-fail-under`
  threshold SHOULD be enforced in CI to prevent coverage
  regression.

- **TC-010** [SHOULD] Place test files in a `tests/`
  directory mirroring the source structure, or co-locate
  with source files using the `test_<module>.py` naming
  convention. Test file naming MUST follow the project's
  established convention consistently.

- **TC-011** [SHOULD] Use `conftest.py` for shared
  fixtures. Do not duplicate fixture definitions across
  test files.

---

## Type Annotations

- **TA-001** [SHOULD] All new public functions and methods
  SHOULD have type annotations on parameters and return
  values.

- **TA-002** [SHOULD] Run a type checker (`mypy`, `pyright`,
  `ty`, or equivalent) and address type errors
  incrementally -- do not disable the type checker globally.

- **TA-003** [SHOULD] Use built-in generics (`list[str]`,
  `dict[str, int]`, `X | None`) for Python 3.10+. For
  Python 3.9 and below, use `typing` module constructs
  (`Optional`, `Union`, `List`, `Dict`).

- **TA-004** [SHOULD] Prefer `Protocol` classes over ABC
  for structural typing where duck typing is the intent.

---

## Documentation Requirements

- **DR-001** [MUST] All public functions, classes, and
  methods MUST have docstrings. Use a consistent docstring
  format (Google-style, NumPy-style, or reStructuredText)
  across the project.

- **DR-002** [MUST] Commit messages MUST use Conventional
  Commits format: `type: description` (e.g., `feat:`,
  `fix:`, `docs:`, `chore:`, `refactor:`).

- **DR-003** [SHOULD] User-visible changes SHOULD be
  recorded in a changelog or release notes following the
  project's established format.

- **DR-004** [SHOULD] Configuration options, environment
  variables, and feature flags SHOULD be documented in
  the project README or a dedicated configuration
  reference.

---

## Custom Rules

<!-- This section is intentionally empty in the canonical pack. Project-specific custom rules belong in python-custom.md -->
