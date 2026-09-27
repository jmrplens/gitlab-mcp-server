#!/usr/bin/env bash
# gitlab-exceptions.sh: keep what GitLab logged as an exception during a run.
#
# Sourced by run-docker-e2e.sh, and by scripts/e2e_gitlab_exceptions_test.py,
# which drives it with a stand-in docker. It defines one function and runs
# nothing, so sourcing it changes no state.
#
# A scenario that meets a 500 learns only that GitLab raised something: the
# answer says "Internal server error", and the exception itself is written to
# /var/log/gitlab/gitlab-rails/exceptions_json.log inside the container, on a
# volume the teardown's `down -v` deletes. The saved view create answered that
# way on every surface of both complete runs of de1ab3b49, and nothing was
# left to say which exception it was, so whether GitLab, client-go's document
# or this server's input set it off could not be settled after the stack was
# gone.

# save_gitlab_exceptions copies GitLab's exceptions log out of the gitlab
# service into the named file, one JSON object per line as GitLab writes it,
# and prints how many it holds. It never fails the run: a stack that never
# started, a container already gone or a log GitLab has not created leaves a
# warning and no file, and a file an earlier run left under that name is
# removed first, since it would read as this run's.
#
#   save_gitlab_exceptions <destination> <compose command...>
save_gitlab_exceptions() {
    local destination="$1" entries
    shift
    rm -f "${destination}" "${destination}.part"
    if ! mkdir -p "$(dirname "${destination}")" ||
        ! "$@" exec -T gitlab cat /var/log/gitlab/gitlab-rails/exceptions_json.log >"${destination}.part" 2>/dev/null; then
        rm -f "${destination}.part"
        echo "WARN: could not read GitLab's exceptions log from the gitlab service; none saved" >&2
        return 0
    fi
    mv "${destination}.part" "${destination}"
    entries="$(wc -l <"${destination}" | tr -d ' ')"
    echo "    GitLab's exceptions log: ${entries} entries, saved to ${destination}"
}
