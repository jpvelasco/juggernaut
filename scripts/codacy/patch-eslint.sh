#!/usr/bin/env bash
# Re-apply Node.js globals after `codacy-cli config reset` overwrites eslint.config.mjs.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
FILE="$ROOT/.codacy/tools-configs/eslint.config.mjs"

if [[ ! -f "$FILE" ]]; then
  echo "patch-eslint: $FILE not found (run codacy-sync first)" >&2
  exit 1
fi

if grep -q 'languageOptions' "$FILE"; then
  echo "patch-eslint: languageOptions already present"
  exit 0
fi

node <<'NODE'
const fs = require("fs");
const path = require("path");
const file = path.join(process.cwd(), ".codacy/tools-configs/eslint.config.mjs");
let src = fs.readFileSync(file, "utf8");
const block = `        languageOptions: {
          globals: {
            require: "readonly",
            module: "readonly",
            __dirname: "readonly",
            __filename: "readonly",
            process: "readonly",
            console: "readonly",
            Buffer: "readonly",
            URL: "readonly",
          },
        },
`;
if (!src.includes("ignores:")) {
  src = src.replace(
    "export default [",
    `export default [\n    {\n        ignores: [\".remember/**\", \".codacy/**\", \"mcps/**\", \"npm/install.js\"],\n    },`
  );
}
src = src.replace(
  /files: \["\*\*\/\*\.js.*?\],\n        rules: \{/s,
  (m) => m.replace("rules: {", block + "        rules: {")
);
fs.writeFileSync(file, src);
console.log("patch-eslint: added Node.js languageOptions");
NODE