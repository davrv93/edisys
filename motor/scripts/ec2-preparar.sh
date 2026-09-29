#!/usr/bin/env bash
# F0 · Prepara un EC2 de 3 GB recién creado (Ubuntu 24.04) para el motor de EDISYS.
# Idempotente: lo puedes correr dos veces. Docker y compose via apt (sin repos externos).
set -euo pipefail
if [ "$(id -u)" -ne 0 ]; then echo "Correr con sudo: sudo ./scripts/ec2-preparar.sh" >&2; exit 1; fi

echo "· paquetes base"
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y -qq docker.io docker-compose-v2 curl >/dev/null
systemctl enable --now docker >/dev/null 2>&1 || true

echo "· swap de 2 GB (red de seguridad del presupuesto §1 del plan)"
if ! swapon --show=NAME,SIZE | grep -q '/swapfile'; then
  fallocate -l 2G /swapfile
  chmod 600 /swapfile
  mkswap /swapfile >/dev/null
  swapon /swapfile
  grep -q '^/swapfile' /etc/fstab || echo '/swapfile none swap sw 0 0' >> /etc/fstab
fi
# El swap es para sobrevivir un pico, no para vivir ahí: presión baja.
sysctl -w vm.swappiness=10 >/dev/null
grep -q '^vm.swappiness' /etc/sysctl.conf || echo 'vm.swappiness=10' >> /etc/sysctl.conf

echo "· límites de compose: el techo de RAM lo pone mem_limit (cgroups v2)"
echo "EC2 listo. Sigue: make descargar && make benchmark-todos"
