#!/usr/bin/env bash
set -euo pipefail
echo "=== Secret Scope Check ==="

# 1. 禁止把本地密钥文件提交到仓库；允许空值模板文件，真实值由下方内容扫描拦截。
tracked_secret_files_pattern='(^|/)\.env($|\.|/)|(^|/).*\.pem$|(^|/).*\.key$|(^|/)id_(rsa|ed25519)$'
allowed_env_templates_pattern='(^|/)\.env\.(example|sample|template|dist)$'
tracked_secret_files="$(git ls-files | grep -E "$tracked_secret_files_pattern" | grep -Ev "$allowed_env_templates_pattern" || true)"
if [ -n "$tracked_secret_files" ]; then
  echo "❌ secret-bearing file is tracked by git"
  echo "$tracked_secret_files"
  exit 1
fi

# 2. 已跟踪文件不得包含常见真实凭据形态。
known_token_pattern='AKIA[0-9A-Z]{16}|LTAI[0-9A-Za-z]{12,}|sk-[A-Za-z0-9]{32,}|ya29\.[A-Za-z0-9_-]+|gh[pousr]_[A-Za-z0-9_]{20,}|github_pat_[A-Za-z0-9_]{20,}'
assignment_pattern='(ACCESS_KEY_SECRET|SECRET_KEY|PASSWORD|PRIVATE_KEY|AUTH_TOKEN|API_TOKEN)[[:space:]]*[:=][[:space:]]*[A-Za-z0-9_./+=-]{16,}'
tracked_secret_matches="$(
  git grep -nI -E "$known_token_pattern|$assignment_pattern" -- \
    ':!go.sum' \
    ':!scripts/secret-scope-check.sh' || true
)"
if [ -n "$tracked_secret_matches" ]; then
  echo "❌ tracked files contain a secret-looking value"
  echo "$tracked_secret_matches"
  exit 1
fi

echo "✅ Secret scope check passed"
