#!/usr/bin/env bash
# fixture-addresses.sh: where the Docker fixture's side services are reached,
# read off the address GitLab is reached at.
#
# Sourced by run-docker-e2e.sh, and by scripts/e2e_fixture_addresses_test.py,
# which drives derive_fixture_addresses with the URLs it has to handle. It
# defines functions and runs nothing, so sourcing it changes no state.

# derive_fixture_addresses reads E2E_DOCKER_GITLAB_URL and fills in each of
# E2E_DOCKER_BITBUCKET_URL, E2E_REGISTRY_EXTERNAL_URL and E2E_BITBUCKET_BIND
# that is not already set. It exports nothing: the caller decides that.
#
# Bitbucket and the registry run beside GitLab, so they are reached on the
# same host. A fixed localhost default sent a remote run's setup script to
# this machine, where nothing listens, and the import test then skipped: the
# run passed and the coverage record came out one action short with nothing
# saying why.
#
# The host is read off the GitLab URL whatever else it carries. The compose
# file publishes GitLab on port 8929 and on no other, so a URL naming another
# port, or none, is a proxy or a tunnel in front of that host, and a trailing
# slash or a path names the same host too. The scheme is not carried over:
# Bitbucket answers plain HTTP on 7990 and the registry on 5050 whatever
# terminates TLS in front of GitLab. A URL no host can be read out of keeps
# the loopback defaults (the compose file's own, for the registry) and says
# so for Bitbucket, rather than stopping a run that may not start it at all.
#
# The bind is chosen from the Bitbucket URL, the derived one or one set by
# hand, by bitbucket_bind below.
derive_fixture_addresses() {
    local gitlab_host=""
    local gitlab_url_re='^[A-Za-z][A-Za-z0-9+.-]*://([^]/?#:[]+|\[[^]/?#]*\])(:[0-9]*)?([/?#].*)?$'
    if [[ "${E2E_DOCKER_GITLAB_URL}" =~ ${gitlab_url_re} ]]; then
        gitlab_host="${BASH_REMATCH[1]}"
    fi
    if [ -z "${E2E_DOCKER_BITBUCKET_URL:-}" ]; then
        if [ -n "${gitlab_host}" ]; then
            E2E_DOCKER_BITBUCKET_URL="http://${gitlab_host}:7990"
        else
            E2E_DOCKER_BITBUCKET_URL="http://localhost:7990"
            echo "run-docker-e2e.sh: WARN no host can be read out of E2E_DOCKER_GITLAB_URL=${E2E_DOCKER_GITLAB_URL};" \
                "Bitbucket is looked for at ${E2E_DOCKER_BITBUCKET_URL}, so set E2E_DOCKER_BITBUCKET_URL if it runs elsewhere" >&2
        fi
    fi
    if [ -z "${E2E_REGISTRY_EXTERNAL_URL:-}" ] && [ -n "${gitlab_host}" ]; then
        E2E_REGISTRY_EXTERNAL_URL="http://${gitlab_host}:5050"
    fi
    if [ -z "${E2E_BITBUCKET_BIND:-}" ]; then
        E2E_BITBUCKET_BIND="$(bitbucket_bind "${E2E_DOCKER_BITBUCKET_URL}")"
    fi
}

# bitbucket_bind prints the address the Bitbucket port is published on for a
# Bitbucket URL: the loopback the URL names when it names one, and 0.0.0.0
# otherwise, since the setup script on this machine has to reach a remote
# container's port and a remote loopback is out of its reach. A URL naming
# this machine by its LAN address is published on 0.0.0.0 too.
#
# The loopback is the one the URL names because the setup script dials that
# address and no other: localhost and 127.* are published on 127.0.0.1, which
# a localhost URL reaches whichever address the name resolves to first, and
# [::1] on [::1], which 127.0.0.1 would leave the script unable to reach. A
# host name is matched without regard to case, as a resolver matches it, so
# LOCALHOST is loopback too.
bitbucket_bind() {
    local restore_case
    restore_case="$(shopt -p nocasematch || true)"
    shopt -s nocasematch
    local bind
    case "$1" in
        *://\[::1\]*) bind="[::1]" ;;
        *://localhost|*://localhost[:/]*|*://127.*) bind=127.0.0.1 ;;
        *) bind=0.0.0.0 ;;
    esac
    eval "${restore_case}"
    echo "${bind}"
}
