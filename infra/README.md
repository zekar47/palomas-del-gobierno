# Infra AWS — palomas-del-gobierno

Stack CloudFormation LearnerLab-safe (región `us-east-1`, cuenta `647810064126`).

## Qué crea el stack `palomas-entorno`

| Recurso lógico | Tipo | Detalle |
|---|---|---|
| `AppSecurityGroup` | `AWS::EC2::SecurityGroup` | En la VPC default (`vpc-02414cab91c6463bb`). Ingress TCP **8080** desde `AllowedCidr` (default `0.0.0.0/0`). **Sin puerto 22.** Egress todo permitido. Tag `Name=palomas-sg`. |
| `AppInstance` | `AWS::EC2::Instance` | Amazon Linux 2023 x86_64 (vía SSM `/aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-x86_64`), tipo `t3.micro`, root gp3 8 GB, `IamInstanceProfile: LabInstanceProfile` (string literal con el nombre del profile existente; esa propiedad es String en CFN). Tag `Name=palomas-app`. |
| `AppEIP` + `AppEIPAssociation` | `AWS::EC2::EIP` + asociación | IP elástica asociada a la instancia. |
| `ArtifactsBucket` | `AWS::S3::Bucket` | Nombre fijo `palomas-artifacts-647810064126`. `BucketOwnerEnforced`, bloqueo público total, versionado suspendido, lifecycle que aborta multipart uploads incompletos a los 7 días. |

El user-data de la instancia (sin instalar nada extra; AL2023 ya trae el SSM agent):
crea el usuario `palomas`, los directorios `/opt/palomas/bin`, `/opt/palomas/uploads`,
`/var/lib/palomas` (chown palomas), genera `/etc/palomas.env` con
`PALOMAS_ADMIN_PASSWORD=<16 bytes hex de /dev/urandom>` (chmod 600), crea la unit
systemd `palomas.service` (`ExecStart=/opt/palomas/bin/palomas --addr :8080
--db /var/lib/palomas/palomas.db --uploads /opt/palomas/uploads`,
`WorkingDirectory=/opt/palomas`, `EnvironmentFile=/etc/palomas.env`,
`Restart=always`, `User=palomas`) y hace `daemon-reload` + `enable`.

> `WorkingDirectory=/opt/palomas` es obligatorio: el binario carga
> `templates/` y sirve `static/` con rutas relativas. Cada deploy sincroniza
> esas carpetas desde S3 (`scripts/instance_deploy.sh` hace
> `aws s3 sync s3://<bucket>/templates|static/ /opt/palomas/...`).

## Desplegar

```bash
aws cloudformation deploy --region us-east-1 --stack-name palomas-entorno \
  --template-file infra/palomas.yml
```

Sin `--capabilities`: el template no contiene ningún recurso IAM.
Parámetros opcionales: `--parameter-overrides InstanceType=t3.micro AllowedCidr=0.0.0.0/0`
(`KeyName` solo si existe un keypair; el SG no abre el 22 de todos modos).

## Ver outputs

```bash
aws cloudformation describe-stacks --region us-east-1 --stack-name palomas-entorno \
  --query 'Stacks[0].Outputs'
```

Outputs: `InstanceId`, `ElasticIP`, `BucketName`, `SecurityGroupId`.
La app queda en `http://<ElasticIP>:8080`.

## Borrar

```bash
aws cloudformation delete-stack --region us-east-1 --stack-name palomas-entorno
aws cloudformation wait stack-delete-complete --region us-east-1 --stack-name palomas-entorno
```

## Nota LearnerLab

- La cuenta es efímera y los roles `iam:CreateRole` / `iam:CreateUser` /
  `iam:CreateOpenIDConnectProvider` están denegados, por eso el template **no**
  contiene ningún recurso `AWS::IAM::*` ni OIDC y reutiliza el
  `LabInstanceProfile` existente (rol `LabRole` con `AmazonSSMManagedInstanceCore`).
- El stack se reconstruye con un solo comando `aws cloudformation deploy ...`;
  la IP elástica **cambia en cada ciclo** (nuevo EIP), así que toma la `ElasticIP`
  de los outputs tras cada despliegue.

## Nota: servicio en retry hasta el primer deploy

El user-data hace `enable` pero **no** `start` de `palomas.service` porque el
binario `/opt/palomas/bin/palomas` aún no existe (llega con el pipeline de
deploy). Con `Restart=always`, systemd reintenta el arranque hasta que el
primer deploy deja el binario. Es normal ver la unidad en fallo/retry al
principio (`systemctl status palomas`).

## Consultar el admin password vía SSM

```bash
INSTANCE_ID=$(aws cloudformation describe-stacks --region us-east-1 \
  --stack-name palomas-entorno --query 'Stacks[0].Outputs[?OutputKey==`InstanceId`].OutputValue' \
  --output text)
aws ssm send-command --region us-east-1 --instance-ids "$INSTANCE_ID" \
  --document-name AWS-RunShellScript \
  --parameters 'commands=["sudo cat /etc/palomas.env"]'
# luego:
aws ssm get-command-invocation --region us-east-1 \
  --command-id <command-id> --instance-id "$INSTANCE_ID"
```
