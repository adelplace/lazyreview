#!/usr/bin/env bash
# Creates the public sandbox repository used to record the demo, with a few
# open pull requests. Safe to run again: existing branches and PRs are kept.
set -euo pipefail

REPO=${DEMO_REPO:-adelplace/lazyreview-demo}
FIX=$(cd "$(dirname "$0")/testdata" && pwd)
WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

if ! gh repo view "$REPO" >/dev/null 2>&1; then
	gh repo create "$REPO" --public --description "Sandbox repository for the lazyreview demo"
fi

cd "$WORK"
git init -q -b main
git remote add origin "https://github.com/$REPO.git"
if git fetch -q origin main 2>/dev/null; then
	git checkout -q -B main FETCH_HEAD
else
	cp -r "$FIX/base/." .
	git add -A
	git commit -q -m "Initial todo API"
	git push -q -u origin main
fi

# branch <name> <fixture> <title> <body> <commit message>...
branch() {
	local name=$1 fixture=$2 title=$3 body=$4
	shift 4
	if [ -n "$(gh pr list -R "$REPO" --head "$name" --state open --json number -q '.[].number')" ]; then
		echo "PR for $name already open"
		return
	fi
	git checkout -q -B "$name" main
	cp -r "$FIX/$fixture/." .
	git add -A
	git commit -q -m "$1"
	git push -q -f origin "$name"
	gh pr create -R "$REPO" --head "$name" --base main --title "$title" --body "$body"
	git checkout -q main
}

branch feat/pagination pagination "Paginate GET /todos" \
	"Adds \`?page=\` and \`?size=\` query parameters to the list endpoint and returns the total count." \
	"Paginate the todo list"
branch fix/race-condition race "Guard the store with a mutex" \
	"Concurrent requests could corrupt the map. The store now uses a \`sync.RWMutex\`." \
	"Make the store safe for concurrent use"
branch docs/readme docs "Document the API endpoints" \
	"Lists every endpoint and adds a curl example." \
	"Document the API"
