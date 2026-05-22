#!/usr/bin/env bash
set -euo pipefail

shopt -s nullglob
files=(test/e2e/suite/*_ee_test.go)

if [ "${#files[@]}" -eq 0 ]; then
  exit 0
fi

if command -v rg >/dev/null 2>&1; then
  rg --no-filename -o '^func Test[[:alnum:]_]+' "${files[@]}" \
    | sed -E 's/^func (Test[[:alnum:]_]+)/\1/' \
    | sort -u \
    | paste -sd '|' -
else
  grep -hE '^func Test[[:alnum:]_]+' "${files[@]}" \
    | sed -E 's/^func (Test[[:alnum:]_]+).*/\1/' \
    | sort -u \
    | paste -sd '|' -
fi
