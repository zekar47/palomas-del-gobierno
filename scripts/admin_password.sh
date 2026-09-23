#!/usr/bin/env bash
# Muestra la contraseña del administrador inicial.
# Se genera una sola vez en el primer arranque (user-data → /etc/palomas.env,
# 16 bytes hex aleatorios) y solo existe dentro de la instancia.
# Uso: ./scripts/admin_password.sh
set -euo pipefail

REGION="${AWS_REGION:-us-east-1}"
STACK="${STACK_NAME:-palomas-entorno}"

INSTANCE_ID=$(aws cloudformation describe-stacks --region "$REGION" --stack-name "$STACK" \
	--query "Stacks[0].Outputs[?OutputKey=='InstanceId'].OutputValue" --output text)

CMD_ID=$(aws ssm send-command --region "$REGION" --instance-ids "$INSTANCE_ID" \
	--document-name AWS-RunShellScript --comment "leer admin password" \
	--parameters '{"commands":["cat /etc/palomas.env"]}' \
	--query 'Command.CommandId' --output text)

for i in $(seq 1 6); do
	sleep 5
	STATUS=$(aws ssm get-command-invocation --region "$REGION" --command-id "$CMD_ID" \
		--instance-id "$INSTANCE_ID" --query Status --output text 2>/dev/null || echo InProgress)
	[[ "$STATUS" = "Success" ]] && break
done
aws ssm get-command-invocation --region "$REGION" --command-id "$CMD_ID" \
	--instance-id "$INSTANCE_ID" --query StandardOutputContent --output text
