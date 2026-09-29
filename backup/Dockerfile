# EDISYS · respaldos: pg_dump diario a las 02:00 de Lima, copia del cubo de Garage y rotación de 30 días.
FROM alpine:3.20
RUN apk add --no-cache bash postgresql16-client rclone tzdata coreutils
ENV TZ=America/Lima RESPALDOS=/respaldos RETENCION_DIAS=30
COPY respaldar.sh restaurar.sh entrada.sh /usr/local/bin/
RUN chmod +x /usr/local/bin/*.sh
VOLUME /respaldos
CMD ["entrada.sh"]
