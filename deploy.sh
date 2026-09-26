#!/usr/bin/env bash
# Deploy or update the Tunnelkey provisioning server with Docker.
#
#   First install:   git clone git@github.com:kalipsers/TunnelKey.git /opt/tunnelkey
#                    /opt/tunnelkey/deploy.sh
#   Update:          /opt/tunnelkey/deploy.sh
#
# Steps: sync the checkout with the remote branch, build the image, (re)start
# the container, wait until it reports healthy, remove dangling images.
#
# Options:
#   -d DIR      checkout directory (default: this script's directory if it is a
#               git checkout, otherwise /opt/tunnelkey; cloned when missing)
#   -b BRANCH   branch to deploy (default: main)
#   -r URL      repository (default: git@github.com:kalipsers/TunnelKey.git)
#   --no-pull   deploy the checkout as it is, without syncing
#   --force     discard local changes to tracked files when syncing
#   -h          help
#
# The same settings can come from TUNNELKEY_DIR, TUNNELKEY_BRANCH, TUNNELKEY_REPO.

set -euo pipefail

REPO_URL="${TUNNELKEY_REPO:-git@github.com:kalipsers/TunnelKey.git}"
BRANCH="${TUNNELKEY_BRANCH:-main}"
DIR="${TUNNELKEY_DIR:-}"
PULL=1
FORCE=0
HEALTH_TIMEOUT=90
CONTAINER=tunnelkey-server

log()  { printf '\033[1;33m==>\033[0m %s\n' "$*"; }
ok()   { printf '\033[1;32m✓\033[0m %s\n' "$*"; }
die()  { printf '\033[1;31m✗ %s\033[0m\n' "$*" >&2; exit 1; }

usage() { sed -n '2,23p' "$0" | sed 's/^# \{0,1\}//'; }

parse_args() {
    while [ $# -gt 0 ]; do
        case "$1" in
            -d) DIR="$2"; shift 2 ;;
            -b) BRANCH="$2"; shift 2 ;;
            -r) REPO_URL="$2"; shift 2 ;;
            --no-pull) PULL=0; shift ;;
            --force) FORCE=1; shift ;;
            -h|--help) usage; exit 0 ;;
            *) die "Unknown option: $1 (see -h)" ;;
        esac
    done
    if [ -z "$DIR" ]; then
        local here
        here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
        if git -C "$here" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
            DIR="$here"
        else
            DIR=/opt/tunnelkey
        fi
    fi
}

check_tools() {
    command -v git >/dev/null || die "git is not installed"
    command -v docker >/dev/null || die "docker is not installed"
    if docker compose version >/dev/null 2>&1; then
        COMPOSE=(docker compose)
    elif command -v docker-compose >/dev/null; then
        COMPOSE=(docker-compose)
    else
        die "Docker Compose is not installed (docker compose plugin or docker-compose)"
    fi
    docker info >/dev/null 2>&1 || die "Cannot talk to the Docker daemon (running? permissions?)"
}

sync_repo() {
    if [ ! -d "$DIR/.git" ]; then
        log "Cloning $REPO_URL ($BRANCH) into $DIR"
        mkdir -p "$(dirname "$DIR")"
        git clone --branch "$BRANCH" "$REPO_URL" "$DIR"
        return
    fi
    [ "$PULL" = 1 ] || { log "Skipping git sync (--no-pull)"; return; }

    log "Syncing $DIR with origin/$BRANCH"
    if ! git -C "$DIR" diff --quiet || ! git -C "$DIR" diff --cached --quiet; then
        [ "$FORCE" = 1 ] || die "Local changes in $DIR — commit/stash them or re-run with --force to discard"
        log "Discarding local changes (--force)"
    fi
    local before after
    before="$(git -C "$DIR" rev-parse --short HEAD)"
    git -C "$DIR" fetch --prune origin "$BRANCH"
    git -C "$DIR" checkout -q "$BRANCH" 2>/dev/null || git -C "$DIR" checkout -q -b "$BRANCH" "origin/$BRANCH"
    git -C "$DIR" reset -q --hard "origin/$BRANCH"
    after="$(git -C "$DIR" rev-parse --short HEAD)"
    if [ "$before" = "$after" ]; then
        ok "Already at $after"
    else
        ok "Updated $before → $after"
        git -C "$DIR" --no-pager log --oneline "$before..$after" | sed 's/^/    /' || true
    fi
}

ensure_env() {
    if [ ! -f "$DIR/.env" ]; then
        cp "$DIR/.env.example" "$DIR/.env"
        chmod 600 "$DIR/.env"
        log "Created $DIR/.env from .env.example — set TUNNELKEY_ADMIN_USER/PASSWORD there for the first admin"
    fi
}

build_and_start() {
    cd "$DIR"
    log "Building image"
    "${COMPOSE[@]}" build --pull
    log "Starting container"
    "${COMPOSE[@]}" up -d --remove-orphans
}

wait_healthy() {
    log "Waiting for the server to become healthy"
    local waited=0 status
    while [ "$waited" -lt "$HEALTH_TIMEOUT" ]; do
        status="$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$CONTAINER" 2>/dev/null || echo missing)"
        case "$status" in
            healthy) ok "Server is healthy"; return ;;
            unhealthy|exited|dead|missing)
                docker logs --tail 50 "$CONTAINER" 2>&1 || true
                die "Container is $status" ;;
        esac
        sleep 3
        waited=$((waited + 3))
    done
    docker logs --tail 50 "$CONTAINER" 2>&1 || true
    die "Server did not become healthy within ${HEALTH_TIMEOUT}s"
}

finish() {
    docker image prune -f >/dev/null 2>&1 || true
    local port bind
    port="$(grep -E '^TUNNELKEY_PORT=' "$DIR/.env" 2>/dev/null | cut -d= -f2 || true)"
    bind="$(grep -E '^TUNNELKEY_BIND=' "$DIR/.env" 2>/dev/null | cut -d= -f2 || true)"
    ok "Deployed $(git -C "$DIR" rev-parse --short HEAD) — admin UI on http://${bind:-0.0.0.0}:${port:-9897}"
    if docker logs "$CONTAINER" 2>&1 | grep -q "No admin account yet"; then
        log "No admin exists yet. Create one with:"
        echo "    docker exec -i $CONTAINER tunnelkey-server admin <username>"
        echo "    (type the password, min. 12 characters, then Enter)"
    fi
}

main() {
    parse_args "$@"
    check_tools
    local self_hash
    self_hash="$(cksum < "${BASH_SOURCE[0]}")"
    sync_repo
    # A pulled update may have changed this script: continue with the new version.
    if [ "$PULL" = 1 ] && [ -z "${TUNNELKEY_REEXEC:-}" ] && [ -f "$DIR/deploy.sh" ] \
        && [ "$(cksum < "$DIR/deploy.sh")" != "$self_hash" ]; then
        log "deploy.sh changed — running the updated version"
        TUNNELKEY_REEXEC=1 exec bash "$DIR/deploy.sh" -d "$DIR" -b "$BRANCH" -r "$REPO_URL" --no-pull
    fi
    ensure_env
    build_and_start
    wait_healthy
    finish
}

# Wrapped in main and exited on the same line so bash has read the whole file
# before `git reset` can rewrite it.
main "$@"; exit $?
