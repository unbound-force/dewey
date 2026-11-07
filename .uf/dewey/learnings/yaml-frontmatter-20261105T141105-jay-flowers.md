---
tag: yaml-frontmatter
author: jay-flowers
category: pattern
created_at: 2026-11-05T14:11:05Z
identity: yaml-frontmatter-20261105T141105-jay-flowers
tier: draft
---

Go's %q (strconv.Quote) emits a double-quoted scalar whose escape set (\" \\ \n \t \r \a \b \v \f \xNN \uNNNN \UNNNNNNNN) is contained in YAML's double-quoted escape set, so markdown frontmatter writers can render quoted values with fmt.Fprintf("key: %q\n", v) and block sequences with "  - %q\n" lines instead of importing a YAML marshaller — as tools/learning_temporal.go does. Pin the guarantee with a test that round-trips the rendered frontmatter through yaml.Unmarshal, including adversarial values with em
