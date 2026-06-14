#!/bin/sh
set -e
cp dist/juggernaut_linux_amd64_v1/juggernaut       npm/packages/juggernaut-bedrock-linux-x64/bin/juggernaut
cp dist/juggernaut_linux_arm64_v8.0/juggernaut     npm/packages/juggernaut-bedrock-linux-arm64/bin/juggernaut
cp dist/juggernaut_darwin_amd64_v1/juggernaut      npm/packages/juggernaut-bedrock-darwin-x64/bin/juggernaut
cp dist/juggernaut_darwin_arm64_v8.0/juggernaut    npm/packages/juggernaut-bedrock-darwin-arm64/bin/juggernaut
cp dist/juggernaut_windows_amd64_v1/juggernaut.exe npm/packages/juggernaut-bedrock-win32-x64/bin/juggernaut.exe
chmod +x npm/packages/juggernaut-bedrock-linux-x64/bin/juggernaut
chmod +x npm/packages/juggernaut-bedrock-linux-arm64/bin/juggernaut
chmod +x npm/packages/juggernaut-bedrock-darwin-x64/bin/juggernaut
chmod +x npm/packages/juggernaut-bedrock-darwin-arm64/bin/juggernaut
