#!/usr/bin/env bash
# Decides whether a pull request needs the full CI jobs.
#
# Reads `git diff --raw --no-renames` output on stdin and prints one word:
#   false  every changed path is a regular .md file under .uf/dewey/learnings/
#          (added, edited or deleted)
#   true   anything else, including no input at all
#
# It fails closed: any line it does not understand means "true". Reasons
# for "true" go to stderr so the CI log shows why the full run happened.
#
# Usage: git diff --raw --no-renames BASE HEAD | scripts/detect-learnings-only.sh
set -euo pipefail

readonly prefix='.uf/dewey/learnings/'

full() {
	echo "full CI: $1" >&2
	echo true
	exit 0
}

seen=0
while IFS= read -r line || [ -n "$line" ]; do
	[ -z "$line" ] && continue
	seen=1

	# Format: ":<src mode> <dst mode> <src sha> <dst sha> <status>\t<path>"
	case "$line" in
	:*$'\t'*) ;;
	*) full "unrecognized diff line: $line" ;;
	esac
	meta=${line%%$'\t'*}
	path=${line#*$'\t'}
	read -r src_mode dst_mode _ <<<"${meta#:}"

	# Symlinks (120000) and submodules (160000) can point outside the
	# learnings directory, so only regular files (or deletions) qualify.
	for mode in "$src_mode" "$dst_mode"; do
		case "$mode" in
		000000 | 100644 | 100755) ;;
		*) full "mode $mode on $path" ;;
		esac
	done

	# Paths git had to quote (unusual characters) start with '"' and fall
	# through to "outside" below.
	case "/$path/" in
	*/../* | */./*) full "relative segment in $path" ;;
	esac
	case "$path" in
	"$prefix"?*.md) ;;
	"$prefix"*) full "not a .md file: $path" ;;
	*) full "outside $prefix: $path" ;;
	esac
done

if [ "$seen" -eq 0 ]; then
	full "no changed files"
fi

echo "every changed file is a Dewey learning" >&2
echo false
