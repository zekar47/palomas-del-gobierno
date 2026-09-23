#!/usr/bin/env bash
# Refresca los secrets AWS del repo con las credenciales vigentes de la
# sesión LearnerLab (expiran en horas; re-corre tras reabrir la lab).
# Uso: ./scripts/refresh_github_aws_secrets.sh
set -euo pipefail
cd "$(dirname "$0")/.."

AK=$(aws configure get aws_access_key_id || true)
SK=$(aws configure get aws_secret_access_key || true)
ST=$(aws configure get aws_session_token || true)

if [[ -z "$AK" ]] || [[ -z "$SK" ]]; then
	echo "no hay credenciales AWS en el perfil actual"
	exit 1
fi

gh secret set AWS_ACCESS_KEY_ID --body "$AK"
gh secret set AWS_SECRET_ACCESS_KEY --body "$SK"
if [[ -n "$ST" ]]; then
	gh secret set AWS_SESSION_TOKEN --body "$ST"
fi
echo "secrets AWS actualizados en el repo"
