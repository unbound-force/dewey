#!/usr/bin/env bash
# Tests for detect-learnings-only.sh. Feeds it `git diff --raw` lines and
# checks the word it prints. Run: bash scripts/detect-learnings-only_test.sh
set -euo pipefail

script="$(cd "$(dirname "$0")" && pwd)/detect-learnings-only.sh"
z=0000000
a=1111111
b=2222222
tab=$'\t'
failures=0

check() {
	local name=$1 want=$2 input=$3 got
	got=$(printf '%s' "$input" | bash "$script" 2>/dev/null)
	if [ "$got" = "$want" ]; then
		echo "ok   $name"
	else
		echo "FAIL $name: want $want, got '$got'"
		failures=$((failures + 1))
	fi
}

L=.uf/dewey/learnings

check "added learning" false \
	":000000 100644 $z $a A${tab}$L/2026-11-24-foo.md"$'\n'
check "added and edited learnings" false \
	":000000 100644 $z $a A${tab}$L/a.md"$'\n'":100644 100644 $a $b M${tab}$L/b.md"$'\n'
check "deleted learning" false \
	":100644 000000 $a $z D${tab}$L/old.md"$'\n'
check "no trailing newline" false \
	":000000 100644 $z $a A${tab}$L/a.md"
check "no input" true ""
check "blank lines only" true $'\n\n'
check "code file" true \
	":100644 100644 $a $b M${tab}cmd/dewey/main.go"$'\n'
check "learning plus code" true \
	":000000 100644 $z $a A${tab}$L/a.md"$'\n'":100644 100644 $a $b M${tab}go.mod"$'\n'
check "workflow file" true \
	":100644 100644 $a $b M${tab}.github/workflows/ci.yml"$'\n'
check "rename from code (delete side)" true \
	":100644 000000 $a $z D${tab}store/store.go"$'\n'":000000 100644 $z $a A${tab}$L/store.go"$'\n'
check "look-alike file" true \
	":000000 100644 $z $a A${tab}.uf/dewey/learnings.md"$'\n'
check "look-alike directory" true \
	":000000 100644 $z $a A${tab}.uf/dewey/learnings-old/a.md"$'\n'
check "directory itself" true \
	":000000 100644 $z $a A${tab}$L/"$'\n'
check "dot-dot segment" true \
	":000000 100644 $z $a A${tab}$L/../../main.go"$'\n'
check "dot segment" true \
	":000000 100644 $z $a A${tab}$L/./a.md"$'\n'
check "symlink in learnings" true \
	":000000 120000 $z $a A${tab}$L/link.md"$'\n'
check "file replaced by symlink" true \
	":100644 120000 $a $b T${tab}$L/a.md"$'\n'
check "submodule in learnings" true \
	":000000 160000 $z $a A${tab}$L/sub"$'\n'
check "executable learning" false \
	":000000 100755 $z $a A${tab}$L/a.md"$'\n'
check "quoted path" true \
	":000000 100644 $z $a A${tab}\"$L/caf\\303\\251.md\""$'\n'
check "line without tab" true \
	":000000 100644 $z $a A $L/a.md"$'\n'
check "name-only line" true \
	"$L/a.md"$'\n'

if [ "$failures" -ne 0 ]; then
	echo "$failures test(s) failed"
	exit 1
fi
echo "all tests passed"
