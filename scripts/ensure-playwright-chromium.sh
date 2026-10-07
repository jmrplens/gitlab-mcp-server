#!/usr/bin/env bash
# Make Playwright's Chromium launchable for the documentation site's build,
# and touch apt only when the browser cannot start without it.
#
# rehype-mermaid renders every Mermaid block to inline SVG during the build by
# launching Playwright's Chromium (mermaid-isomorphic calls chromium.launch()
# with no options), so the build needs the browser build the lockfile's
# Playwright pins and the system libraries that browser links against. Both
# used to come from `playwright install --with-deps chromium`, and the second
# half is the one that failed.
#
# --with-deps runs apt-get update and apt-get install as root on every run,
# whatever the cache holds. On the GitHub-hosted Ubuntu 24.04 image that
# install adds no library Chromium needs, because the image ships Google
# Chrome and its package depends on all of them: it added nine font packages
# and upgraded whatever was pending. And nothing bounded it. On 2026-10-07 the
# Azure mirror stopped answering, apt fell back to archive.ubuntu.com, read
# its release files and then printed nothing until three site build jobs were
# killed by their 20-minute timeout, every cache having hit.
#
# So the browser is installed alone, which a cache hit satisfies without a
# download, and then launched the way the build launches it. Only when that
# fails are the system libraries installed, with `playwright install-deps
# chromium`, and the launch tried again; if it still fails, the step says why
# and stops instead of waiting for the job's timeout.
#
# The fonts `--with-deps` used to add do not reach the published diagrams.
# Every one is drawn in `arial, sans-serif`, and arial resolves to Liberation
# Sans from the fonts-liberation package Google Chrome depends on. All 52
# diagrams the site had on 2026-10-07, rendered with the site's rehype-mermaid
# options in an image built the way the runner's is, came out byte-identical
# with and without those fonts. A character Liberation Sans lacks would fall
# back to a different font without them; no diagram uses one.
#
# The bound on install-deps is timeout(1) around each of a few attempts, and
# not apt's own settings. The runner image already sets those (one retry and a
# 15-second timeout, in an apt.conf.d file it names to sort after every other
# one), and they were in force during the hang: apt gave up on the Azure
# mirror 15 seconds after asking it (30 in the first of the three jobs).
# apt's Timeout is an inactivity timer on its connections, so a mirror that
# keeps a transfer open without finishing it never trips it, whatever it is
# set to.
#
# The timeout runs as root, around Playwright's CLI rather than around pnpm.
# When it is not root, install-deps hands sudo
# `sh -c "apt-get update&& apt-get install ..."`. With no terminal, as on a
# runner, every one of those processes stays in the process group a timeout
# signals; what stops the signal is that sudo passes it on to the one process
# it started, that shell, and to nothing the shell starts, and that the
# runner's user cannot signal a root process such as apt-get directly. So a
# timeout run as the runner's user ended pnpm, node, sudo and the shell and
# left apt-get running: tried against a stalled mirror, apt-get update
# outlived the first attempt and held the lock that failed the second one at
# once. Run as root, install-deps starts that shell itself, in the timeout's
# process group, and a timeout that is root reaches every process in it, so
# the attempt ends with everything it started.
#
# An attempt stopped while dpkg was unpacking leaves dpkg marked interrupted,
# and every apt-get after it refuses to run until `dpkg --configure -a` has
# finished the job; a plain Ubuntu image on a slow link reached that state and
# failed its second attempt at once. So each retry is preceded by that
# command, as root and under a bound of its own, which does nothing when
# nothing was left half done.
#
# The bounds are sized from the step's healthy runs on the runner, which
# installed fonts only, and from what a full install costs: 103 packages and
# 98 MB on a plain Ubuntu 24.04, which at the 600 kB/s the mirror served
# during the slow runs of 2026-10-07 is under three minutes of download,
# and an attempt is given five.
#
# Usage, from the site directory (the one whose node_modules holds playwright):
#
#   ../scripts/ensure-playwright-chromium.sh
#
# The bounds can be shortened for a test; the defaults are what CI uses:
#
#   PLAYWRIGHT_BROWSER_SECONDS       the browser install (default 120)
#   PLAYWRIGHT_LAUNCH_SECONDS        each launch check (default 60)
#   PLAYWRIGHT_DEPS_ATTEMPTS         install-deps attempts (default 2)
#   PLAYWRIGHT_DEPS_ATTEMPT_SECONDS  each install-deps attempt (default 300)
#   PLAYWRIGHT_DPKG_REPAIR_SECONDS   the dpkg repair before a retry (default 60)

set -euo pipefail

browser_seconds=${PLAYWRIGHT_BROWSER_SECONDS:-120}
launch_seconds=${PLAYWRIGHT_LAUNCH_SECONDS:-60}
attempts=${PLAYWRIGHT_DEPS_ATTEMPTS:-2}
attempt_seconds=${PLAYWRIGHT_DEPS_ATTEMPT_SECONDS:-300}
repair_seconds=${PLAYWRIGHT_DPKG_REPAIR_SECONDS:-60}

# What rehype-mermaid does with the browser, short of rendering a diagram:
# chromium.launch() with no options, a page, content in it. It is resolved from
# the working directory's node_modules, so it is the Playwright the lockfile
# pins and the browser build that Playwright asks for. A missing library fails
# the launch itself, and the message names the library. The template literal
# is JavaScript's, so the single quotes are meant.
# shellcheck disable=SC2016
launch_check='
import { chromium } from "playwright";
try {
  const browser = await chromium.launch();
  try {
    const page = await browser.newPage();
    await page.setContent("<p>launched</p>");
    const text = await page.textContent("p");
    if (text !== "launched") {
      throw new Error(`the page rendered ${JSON.stringify(text)}`);
    }
  } finally {
    await browser.close();
  }
} catch (error) {
  console.error(error.message);
  process.exit(1);
}
'

launches() {
  timeout --kill-after=10 "$launch_seconds" node --input-type=module -e "$launch_check"
}

# Says how a launch check that returned the given status failed: it ran out
# of time, which is not a missing library, or Chromium refused to start, whose
# reason the check printed above.
launch_failure() {
  if [ "$1" -eq 124 ] || [ "$1" -eq 137 ]; then
    echo "did not launch within ${launch_seconds} s"
  else
    echo "does not launch"
  fi
}

# Runs a command as root: directly when this already is root, through sudo
# otherwise, which is how a GitHub-hosted runner grants it.
as_root() {
  if [ "$(id -u)" -eq 0 ]; then
    "$@"
  else
    sudo -- "$@"
  fi
}

# install-deps as root, inside its timeout. node is named by its path, because
# sudo replaces PATH with its secure_path.
install_deps() {
  local node_bin
  node_bin=$(command -v node)
  as_root timeout --kill-after=15 "$attempt_seconds" \
    "$node_bin" node_modules/playwright/cli.js install-deps chromium
}

# Finishes whatever dpkg was left doing by an attempt the timeout stopped.
repair_dpkg() {
  as_root timeout --kill-after=10 "$repair_seconds" dpkg --configure -a
}

echo "Installing Playwright's Chromium build; a cache hit leaves nothing to download."
status=0
timeout --kill-after=10 "$browser_seconds" pnpm exec playwright install chromium || status=$?
if [ "$status" -ne 0 ]; then
  echo "::error title=Playwright Chromium::playwright install chromium did not finish within ${browser_seconds} s (exit status ${status}). The site build renders its Mermaid diagrams with this browser and cannot run without it."
  exit 1
fi

status=0
launches || status=$?
if [ "$status" -eq 0 ]; then
  echo "Chromium launches with the libraries this machine already has; apt was not used."
  exit 0
fi

echo "::warning title=Playwright Chromium::Chromium $(launch_failure "$status") with the libraries this machine has, so they are installed with apt: at most ${attempts} attempts of ${attempt_seconds} s each."

for ((attempt = 1; attempt <= attempts; attempt++)); do
  if [ "$attempt" -gt 1 ]; then
    echo "dpkg --configure -a, to finish what attempt $((attempt - 1)) may have left half done"
    status=0
    repair_dpkg || status=$?
    if [ "$status" -ne 0 ]; then
      echo "dpkg --configure -a exited with status ${status}."
    fi
  fi
  echo "playwright install-deps chromium, attempt ${attempt} of ${attempts}"
  status=0
  install_deps || status=$?
  if [ "$status" -eq 0 ]; then
    status=0
    launches || status=$?
    if [ "$status" -eq 0 ]; then
      echo "Chromium launches after its system libraries were installed."
      exit 0
    fi
    # The packages are there and the browser still does not start: another
    # attempt would install nothing new, and the failure is the browser's,
    # not apt's.
    echo "::error title=Playwright Chromium::playwright install-deps chromium installed Chromium's system libraries, and Chromium still $(launch_failure "$status"). The launch check's output, if it printed any, is above. The site build renders its Mermaid diagrams with this browser and cannot run without it."
    exit 1
  fi
  if [ "$status" -eq 124 ] || [ "$status" -eq 137 ]; then
    echo "Attempt ${attempt} was stopped after ${attempt_seconds} s."
  else
    echo "Attempt ${attempt} exited with status ${status}."
  fi
done

# Reached only when every attempt failed: an attempt that installed the
# libraries ends the script above, whether the browser then launched or not.
echo "::error title=Playwright Chromium::Chromium does not launch, and its system libraries could not be installed with apt (${attempts} attempts of at most ${attempt_seconds} s each). The site build renders its Mermaid diagrams with this browser and cannot run without it."
exit 1
