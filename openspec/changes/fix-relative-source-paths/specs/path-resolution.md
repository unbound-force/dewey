## MODIFIED Requirements

### Requirement: Source Path Resolution

All source types with a `path` config field MUST resolve relative paths against the vault basePath. Only absolute paths are used for filesystem operations.

Previously: Only `path: "."` was resolved. Other relative paths were used as-is.

#### Scenario: Relative sibling path
- **GIVEN** a disk source with `path: "../gaze"` and basePath `/Users/x/unbound-force/dewey`
- **WHEN** the source is created
- **THEN** the resolved path is `/Users/x/unbound-force/gaze`

#### Scenario: Dot path (existing behavior)
- **GIVEN** a disk source with `path: "."` and basePath `/Users/x/unbound-force/dewey`
- **WHEN** the source is created
- **THEN** the resolved path is `/Users/x/unbound-force/dewey`

#### Scenario: Absolute path (passthrough)
- **GIVEN** a disk source with `path: "/opt/data"` and basePath `/Users/x/dewey`
- **WHEN** the source is created
- **THEN** the resolved path is `/opt/data` (unchanged)

#### Scenario: Code source relative path
- **GIVEN** a code source with `path: "../replicator"` and basePath `/Users/x/unbound-force/dewey`
- **WHEN** the source is created
- **THEN** the resolved path is `/Users/x/unbound-force/replicator`
