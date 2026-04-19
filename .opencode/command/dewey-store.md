---
description: Store pasted text as a Dewey learning with intelligent tag and category suggestions.
---

# Command: /dewey-store

## Description

Store ad-hoc knowledge (Slack DMs, meeting notes, decisions,
observations) as a Dewey learning. Supports three modes:
fully specified, suggested (agent analyzes text and proposes
tag/category), and extract (breaks a long conversation into
multiple learnings).

## Usage

```
/dewey-store --tag auth-design --category decision
Sarah confirmed we need OAuth2 + SAML for enterprise.

/dewey-store
Just talked to the team — we're switching to PostgreSQL
for the analytics pipeline. MySQL can't handle the
partitioning we need.

/dewey-store --extract
[paste a long Slack conversation or meeting transcript]
```

## Instructions

### 1. Parse Input

Read the user's message after `/dewey-store`.

**Check for flags**:
- `--tag <value>`: Topic tag for the learning
- `--category <value>`: One of `decision`, `pattern`,
  `gotcha`, `context`, `reference`
- `--extract`: Enable multi-learning extraction mode

Everything after the flags (or the entire message if no
flags) is the **text to store**.

If no text is provided, ask:
> "What knowledge would you like to store? Paste the
> text (Slack message, meeting note, observation, etc.)"

### 2. Determine Mode

**Mode A: Fully Specified** (both `--tag` and `--category`
provided)

Call the Dewey MCP tool `store_learning` immediately:
```
store_learning(
  tag: "<provided tag>",
  category: "<provided category>",
  information: "<pasted text>"
)
```
Skip to Step 5 (Post-Store).

**Mode B: Suggested** (no `--tag` or no `--category`)

Proceed to Step 3 (Analyze and Suggest).

**Mode C: Extract** (`--extract` flag present)

Proceed to Step 4 (Multi-Learning Extraction).

### 3. Analyze and Suggest (Mode B)

Read the pasted text and identify:

**Tag suggestion**: Identify the primary topic. Suggest
2-3 tags ranked by specificity:
1. Most specific (e.g., `oauth2-timeout`)
2. Broader topic (e.g., `authentication`)
3. Project-level (e.g., `sprint-review`)

Present as:

> **Suggested tag**: `authentication` — this text
> discusses auth method selection (OAuth2 vs SAML)
>
> Other options: `enterprise-requirements`,
> `auth-design`
>
> Your choice (or type your own):

**Category suggestion**: Classify the text based on
content patterns:
- Contains "decided", "agreed", "confirmed", "will use",
  "switching to", "going with" → suggest `decision`
- Contains "watch out", "gotcha", "careful", "trap",
  "bug", "issue" → suggest `gotcha`
- Contains "pattern", "approach", "technique", "always",
  "best practice" → suggest `pattern`
- Contains a URL, "see also", "reference", "docs at" →
  suggest `reference`
- Default → suggest `context`

Present as:

> **Suggested category**: `decision` — this contains a
> confirmed choice about database technology
>
> Options: `decision` | `pattern` | `gotcha` | `context`
> | `reference`
>
> Your choice (or press enter for `decision`):

After the user confirms (or says "yes", "ok", "looks
good", or presses enter), call `store_learning` with
the confirmed values.

### 4. Multi-Learning Extraction (Mode C)

Read the full pasted text and identify distinct pieces
of knowledge — decisions, facts, observations, action
items. Each should be a self-contained statement that
makes sense without the surrounding conversation.

Present the proposed extractions as a numbered list:

> I found **N** distinct items in this text:
>
> 1. **`auth-design`** (decision): "Team decided to
>    switch from API keys to OAuth2 for scoping support"
> 2. **`deployment`** (context): "Production deploy
>    window is Tuesdays and Thursdays 2-4pm ET"
> 3. **`auth-design`** (decision): "OAuth2 timeout set
>    to 60 seconds based on beta feedback"
>
> Store all? Or type numbers to remove (e.g., "remove 2")
> or edit (e.g., "2: tag=deploy-schedule"):

After confirmation, call `store_learning` for each item.
Display each returned identity.

### 5. Post-Store

After successful storage, display:

```
Stored: {tag}-{sequence}
```

If multiple items were stored (extract mode):
```
Stored:
  auth-design-4
  deployment-1
  auth-design-5
```

Then suggest:

> Run `/dewey-compile` to synthesize this with existing
> knowledge, or continue working.
