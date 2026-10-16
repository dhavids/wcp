#!/bin/bash
# Promote dev into main: validates dev is clean and passing, then
# fast-forwards main to dev's tip. Never pushes on its own -- pass
# --push to also push main to origin afterward.
#
# Unlike owrap's own deploy.py, this doesn't filter any files out of
# the promoted snapshot -- wcp has no dev-only tooling (tests ship
# fine in the public repo), so main is just dev's history, not a
# separate filtered export.
set -euo pipefail

DEV_BRANCH="dev"
MAIN_BRANCH="main"

push=0
for arg in "$@"; do
    case "$arg" in
        --push) push=1 ;;
        *)
            echo "promote.sh: unknown argument '$arg'" >&2
            exit 1
            ;;
    esac
done

current_branch="$(git rev-parse --abbrev-ref HEAD)"
if [[ "$current_branch" != "$DEV_BRANCH" ]]; then
    echo "promote.sh: must be run from '$DEV_BRANCH' (currently on '$current_branch')" >&2
    exit 1
fi

if [[ -n "$(git status --porcelain)" ]]; then
    echo "promote.sh: '$DEV_BRANCH' has uncommitted changes" >&2
    exit 1
fi

echo "promote.sh: running go vet + go test..."
go vet ./...
go test ./...

target_commit="$(git rev-parse --short HEAD)"
echo "promote.sh: fast-forwarding '$MAIN_BRANCH' to '$DEV_BRANCH' ($target_commit)"
git branch -f "$MAIN_BRANCH" "$DEV_BRANCH"

if [[ "$push" -eq 1 ]]; then
    echo "promote.sh: pushing '$MAIN_BRANCH' to origin"
    git push origin "$MAIN_BRANCH"
else
    echo "promote.sh: done locally -- pass --push to also push '$MAIN_BRANCH' to origin"
fi

git checkout "$DEV_BRANCH" >/dev/null
