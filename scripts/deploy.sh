#!/usr/bin/env bash
# Despliegue local (el método primario en LearnerLab: las credenciales de la
# lab viven en esta máquina y rotan; GitHub Actions usa copias vía secrets).
# Sube binario + script de instancia + templates/static a S3 y ejecuta UN solo
# comando SSM que converge la instancia y verifica salud.
# Uso: ./scripts/deploy.sh
set -euo pipefail
cd "$(dirname "$0")/.."

REGION="${AWS_REGION:-us-east-1}"
STACK="${STACK_NAME:-palomas-entorno}"

echo "== outputs del stack $STACK =="
INSTANCE_ID=$(aws cloudformation describe-stacks --region "$REGION" --stack-name "$STACK" \
	--query "Stacks[0].Outputs[?OutputKey=='InstanceId'].OutputValue" --output text)
BUCKET=$(aws cloudformation describe-stacks --region "$REGION" --stack-name "$STACK" \
	--query "Stacks[0].Outputs[?OutputKey=='BucketName'].OutputValue" --output text)
EIP=$(aws cloudformation describe-stacks --region "$REGION" --stack-name "$STACK" \
	--query "Stacks[0].Outputs[?OutputKey=='ElasticIP'].OutputValue" --output text)
# Sin dominio propio: <ip-con-guiones>.sslip.io resuelve a la EIP (ver instance_deploy.sh).
SITE="$(echo "$EIP" | tr '.' '-')".sslip.io
APP_URL="https://$SITE"
echo "instancia: $INSTANCE_ID | bucket: $BUCKET | url: $APP_URL"

echo "== compilar linux/amd64 =="
GOOS=linux GOARCH=amd64 go build -o /tmp/opencode/palomas-linux-amd64 .

echo "== subir artefactos a s3://$BUCKET =="
aws s3 cp /tmp/opencode/palomas-linux-amd64 "s3://$BUCKET/palomas-linux-amd64" --region "$REGION"
aws s3 cp scripts/instance_deploy.sh "s3://$BUCKET/instance_deploy.sh" --region "$REGION"
aws s3 sync templates/ "s3://$BUCKET/templates/" --delete --region "$REGION"
aws s3 sync static/ "s3://$BUCKET/static/" --delete --region "$REGION"

echo "== desplegar vía SSM =="
CMD_ID=$(aws ssm send-command --region "$REGION" --instance-ids "$INSTANCE_ID" \
	--document-name AWS-RunShellScript --comment "deploy palomas local" \
	--parameters "{\"commands\":[\"aws s3 cp s3://$BUCKET/instance_deploy.sh /tmp/instance_deploy.sh && bash /tmp/instance_deploy.sh $BUCKET\"]}" \
	--query 'Command.CommandId' --output text)
echo "command: $CMD_ID"
STATUS="Pending"
for i in $(seq 1 36); do
	sleep 5
	STATUS=$(aws ssm get-command-invocation --region "$REGION" --command-id "$CMD_ID" \
		--instance-id "$INSTANCE_ID" --query Status --output text 2>/dev/null || echo "InProgress")
	echo "intento $i: $STATUS"
	[ "$STATUS" = "Success" ] && break
	case "$STATUS" in
	InProgress|Pending|Delayed) ;;
	*)
		aws ssm get-command-invocation --region "$REGION" --command-id "$CMD_ID" --instance-id "$INSTANCE_ID"
		exit 1
		;;
	esac
done
[ "$STATUS" = "Success" ] || { echo "SSM no llegó a Success"; exit 1; }

echo "== health check externo =="
for i in $(seq 1 12); do
	sleep 5
	if curl -fsS "$APP_URL/" -o /dev/null; then
		echo "salud OK: $APP_URL"
		exit 0
	fi
done
echo "health check falló: $APP_URL"
exit 1
