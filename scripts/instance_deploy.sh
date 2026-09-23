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
ExecStart=/opt/palomas/bin/palomas --addr :8080 --db /var/lib/palomas/palomas.db --uploads /opt/palomas/uploads --cookie-secure
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
echo "palomas en línea (http local)"

# --- Caddy: terminador TLS con certificado Let's Encrypt automático ---
# Sin dominio propio: se usa <ip-con-guiones>.sslip.io, que resuelve a la EIP.
if ! command -v caddy >/dev/null 2>&1; then
	echo "instalando caddy..."
	CADDY_TAG=$(python3 -c "import json,urllib.request; print(json.load(urllib.request.urlopen('https://api.github.com/repos/caddyserver/caddy/releases/latest', timeout=30))['tag_name'])")
	CADDY_VER="${CADDY_TAG#v}"
	cd /tmp
	rm -f caddy.tgz # resto de un intento anterior con nombre distinto
	curl -fsSL -o "caddy_${CADDY_VER}_linux_amd64.tar.gz" "https://github.com/caddyserver/caddy/releases/download/${CADDY_TAG}/caddy_${CADDY_VER}_linux_amd64.tar.gz"
	curl -fsSL -o caddy_checks.txt "https://github.com/caddyserver/caddy/releases/download/${CADDY_TAG}/caddy_${CADDY_VER}_checksums.txt"
	grep "caddy_${CADDY_VER}_linux_amd64.tar.gz" caddy_checks.txt | sha512sum -c -
	tar xzf "caddy_${CADDY_VER}_linux_amd64.tar.gz" caddy
	install -m 0755 caddy /usr/local/bin/caddy
	rm -f "caddy_${CADDY_VER}_linux_amd64.tar.gz" caddy_checks.txt caddy
fi
command -v setcap >/dev/null 2>&1 || dnf install -y libcap
setcap cap_net_bind_service=+ep /usr/local/bin/caddy
id caddy >/dev/null 2>&1 || useradd -r -s /bin/false -d /var/lib/caddy caddy
mkdir -p /etc/caddy /var/lib/caddy /var/log/caddy
chown -R caddy:caddy /var/lib/caddy /var/log/caddy

MD_TOKEN=$(curl -fsS -m 10 -X PUT -H "X-aws-ec2-metadata-token-ttl-seconds: 60" http://169.254.169.254/latest/api/token)
PUBIP=$(curl -fsS -m 10 -H "X-aws-ec2-metadata-token: $MD_TOKEN" http://169.254.169.254/latest/meta-data/public-ipv4)
SITE="${PUBIP//./-}.sslip.io"
echo "sitio https: $SITE"

cat > /etc/caddy/Caddyfile <<EOF
$SITE {
	encode gzip
	reverse_proxy 127.0.0.1:8080
}
EOF

cat > /etc/systemd/system/caddy.service <<'UNIT'
[Unit]
Description=Caddy (TLS para palomas)
After=network-online.target
Wants=network-online.target
[Service]
User=caddy
Group=caddy
ExecStart=/usr/local/bin/caddy run --config /etc/caddy/Caddyfile --adapter caddyfile
ExecReload=/usr/local/bin/caddy reload --config /etc/caddy/Caddyfile --adapter caddyfile
Restart=always
RestartSec=5
[Install]
WantedBy=multi-user.target
UNIT
systemctl daemon-reload
systemctl enable --now caddy.service

# La primera emisión Let's Encrypt puede tardar ~1 min (desafío ACME en :80).
OK=0
for i in $(seq 1 24); do
	sleep 10
	if curl -fsS "https://$SITE/" -o /dev/null; then
		OK=1
		break
	fi
done
if [ "$OK" != "1" ]; then
	systemctl status caddy --no-pager || true
	journalctl -u caddy --no-pager -n 40 || true
	exit 1
fi
echo "https en línea: https://$SITE"
