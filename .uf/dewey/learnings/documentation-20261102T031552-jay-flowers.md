---
tag: documentation
author: jay-flowers
category: pattern
created_at: 2026-11-02T03:15:52Z
identity: documentation-20261102T031552-jay-flowers
tier: draft
---

When one tool in a PR gains shorthand name resolution (dewey promote: bare name resolves against compiled/ then learning/, documented in the README tool table, README CLI section, MCP tool description, and jsonschema) while a newly-unified enumeration surface displays a different name form (list_pages lists compiled articles under their bare OriginalName topic; get_page requires the canonical compiled/<topic>), the documentation asymmetry invites round-trip failures: agents copy the displayed name into the read tool and receive not-found — the same failure mode dewey #91 recorded for promote before it gained shorthand. Rule: documentation for a unified read surface must state the name form each tool accepts, and documentation reviewers should round-trip check the docs (enumerate → displayed name → documented lookup name) exactly as behavior reviewers round-trip check the tools.
