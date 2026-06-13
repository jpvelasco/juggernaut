#!/usr/bin/env bash
# Run Codacy CLI tools plus optional standalone linters for server-only tools.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

FAIL=0
CODACY_CLI="${CODACY_CLI:-codacy-cli}"
run_codacy() {
  if command -v codacy-cli &>/dev/null; then
    codacy-cli "$@"
  else
    bash <(curl -Ls https://raw.githubusercontent.com/codacy/codacy-cli-v2/main/codacy-cli.sh) "$@"
  fi
}

if [[ ! -f .codacy/codacy.yaml ]]; then
  echo "Missing .codacy/codacy.yaml — run: make codacy-sync (requires CODACY_API_TOKEN)" >&2
  exit 1
fi

echo "==> codacy-cli analyze (all bundled tools)"
ANALYZE_LOG="$(mktemp)"
if ! run_codacy analyze 2>&1 | tee "$ANALYZE_LOG"; then
  echo "FAIL: codacy-cli analyze" >&2
  FAIL=1
fi
if grep -qE '✖ [1-9]|:[0-9]+:[A-Za-z][A-Za-z0-9_-]*:|files: [1-9][0-9]* finding' "$ANALYZE_LOG"; then
  echo "FAIL: codacy-cli reported issues (see above)" >&2
  FAIL=1
fi
if grep -q 'Tool failed to run' "$ANALYZE_LOG"; then
  echo "FAIL: a Codacy CLI tool failed to run" >&2
  FAIL=1
fi
rm -f "$ANALYZE_LOG"

if command -v shellcheck &>/dev/null; then
  echo "==> shellcheck scripts/*.sh"
  if ! shellcheck scripts/*.sh; then
    echo "FAIL: shellcheck" >&2
    FAIL=1
  fi
else
  echo "SKIP: shellcheck not installed (apt install shellcheck / brew install shellcheck)"
fi

if command -v markdownlint &>/dev/null; then
  echo "==> markdownlint (tracked markdown)"
  MD_FILES=()
  while IFS= read -r f; do MD_FILES+=("$f"); done < <(git ls-files '*.md')
  if [[ ${#MD_FILES[@]} -gt 0 ]]; then
    if ! markdownlint -c .markdownlint.json "${MD_FILES[@]}"; then
      echo "FAIL: markdownlint" >&2
      FAIL=1
    fi
  fi
elif command -v npx &>/dev/null; then
  echo "==> markdownlint via npx"
  MD_FILES=()
  while IFS= read -r f; do MD_FILES+=("$f"); done < <(git ls-files '*.md')
  if [[ ${#MD_FILES[@]} -gt 0 ]]; then
    if ! npx --yes markdownlint-cli2 --config .markdownlint.json "${MD_FILES[@]}"; then
      echo "FAIL: markdownlint" >&2
      FAIL=1
    fi
  fi
else
  echo "SKIP: markdownlint not available"
fi

if command -v pwsh &>/dev/null; then
  echo "==> PSScriptAnalyzer scripts/*.ps1"
  if ! pwsh -NoProfile -Command \
    '$e = Invoke-ScriptAnalyzer -Path scripts/*.ps1 -Severity Error; if ($e) { $e | Format-Table; exit 1 }'; then
    echo "FAIL: PSScriptAnalyzer" >&2
    FAIL=1
  fi
else
  echo "SKIP: pwsh not installed (PSScriptAnalyzer runs on Codacy cloud only otherwise)"
fi

if command -v python3 &>/dev/null && python3 -c 'import json' &>/dev/null; then
  echo "==> JSON syntax (Jackson Linter parity) for tracked *.json"
  JSON_FAIL=0
  while IFS= read -r f; do
    if ! python3 -m json.tool "$f" >/dev/null 2>&1; then
      echo "  invalid JSON: $f" >&2
      JSON_FAIL=1
    fi
  done < <(git ls-files '*.json' ':!:mcps/**')
  if [[ "$JSON_FAIL" -ne 0 ]]; then
    echo "FAIL: JSON validation" >&2
    FAIL=1
  fi
fi

if [[ -n "${CODACY_API_TOKEN:-}" ]] && command -v codacy &>/dev/null; then
  echo "==> codacy cloud issues overview (dashboard ground truth)"
  codacy issues gh jpvelasco juggernaut --overview || true
elif [[ -n "${CODACY_API_TOKEN:-}" ]] && command -v npx &>/dev/null; then
  echo "==> codacy cloud issues overview (via npx)"
  npx --yes @codacy/codacy-cloud-cli issues gh jpvelasco juggernaut --overview || true
fi

if [[ "$FAIL" -ne 0 ]]; then
  echo "" >&2
  echo "Codacy full analysis failed. Fix findings above before pushing." >&2
  exit 1
fi

echo "Codacy full analysis passed."