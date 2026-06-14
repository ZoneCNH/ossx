#!/usr/bin/env bash
set -euo pipefail
echo "=== Secret Scope Check ==="

# 1. 禁止 .env 被提交
if git ls-files | grep -q '^\.env$'; then
  echo "❌ .env is tracked by git — remove it"
  exit 1
fi

# 2. .env.example 不得包含真实密钥模式
if [ -f .env.example ]; then
  if grep -qE '(AKIA|sk-[A-Za-z0-9]{32,}|ya29\.|ghp_|gho_|ghu_|ghs_|github_pat_)' .env.example 2>/dev/null; then
    echo "❌ .env.example contains real secret pattern"
    exit 1
  fi
fi

echo "✅ Secret scope check passed"
