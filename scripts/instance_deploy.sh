#!/usr/bin/env bash
# Corre EN LA INSTANCIA (vía SSM). $1 = bucket S3 de artefactos.
# Idempotente: deja binario + templates/static + unit systemd convergidos y verifica salud.
set -euo pipefail
BUCKET="${1:?bucket requerido}"

aws s3 cp "s3://$BUCKET/palomas-linux-amd64" /opt/palomas/bin/palomas
chmod +x /opt/palomas/bin/palomas
aws s3 sync "s3://$BUCKET/templates/" /opt/palomas/templates/ --delete
aws s3 sync "s3://$BUCKET/static/" /opt/palomas/static/ --delete
chown -R palomas:palomas /opt/palomas

cat > /etc/systemd/system/palomas.service <<'UNIT'
[Unit]
Description=palomas-del-gobierno
After=network-online.target
Wants=network-online.target
[Service]
Type=simple
User=palomas
WorkingDirectory=/opt/palomas
EnvironmentFile=/etc/palomas.env
ExecStart=/opt/palomas/bin/palomas --addr :8080 --db /var/lib/palomas/palomas.db --uploads /opt/palomas/uploads
Restart=always
RestartSec=5
[Install]
WantedBy=multi-user.target
UNIT
systemctl daemon-reload
systemctl enable --now palomas.service

for i in $(seq 1 6); do
	sleep 5
	systemctl is-active --quiet palomas && break
done
if ! systemctl is-active --quiet palomas; then
	systemctl status palomas --no-pager || true
	journalctl -u palomas --no-pager -n 30 || true
	exit 1
fi
curl -fsS http://localhost:8080/ -o /dev/null
echo "palomas en línea"
