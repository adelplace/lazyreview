#!/usr/bin/env bash
# Puts the sandbox PRs back in their initial state before recording: no
# pending review of yours and every file unviewed.
set -euo pipefail

REPO=${DEMO_REPO:-adelplace/lazyreview-demo}
OWNER=${REPO%/*}
NAME=${REPO#*/}

gh api graphql -F owner="$OWNER" -F name="$NAME" -f query='
query($owner: String!, $name: String!) {
  repository(owner: $owner, name: $name) {
    pullRequests(states: OPEN, first: 20) {
      nodes {
        id
        files(first: 100) { nodes { path viewerViewedState } }
        reviews(states: PENDING, first: 10) { nodes { id viewerDidAuthor } }
      }
    }
  }
}' --jq '.data.repository.pullRequests.nodes[] as $pr |
  ($pr.reviews.nodes[] | select(.viewerDidAuthor) | "review\t\(.id)"),
  ($pr.files.nodes[] | select(.viewerViewedState != "UNVIEWED") | "file\t\($pr.id)\t\(.path)")' |
while IFS=$'\t' read -r kind id path; do
	case $kind in
	review)
		gh api graphql -f id="$id" -f query='
mutation($id: ID!) { deletePullRequestReview(input: {pullRequestReviewId: $id}) { clientMutationId } }' >/dev/null
		echo "deleted pending review $id"
		;;
	file)
		gh api graphql -f id="$id" -f path="$path" -f query='
mutation($id: ID!, $path: String!) { unmarkFileAsViewed(input: {pullRequestId: $id, path: $path}) { clientMutationId } }' >/dev/null
		echo "unviewed $path"
		;;
	esac
done
