# Nightly backup job for Railway (cron service). See docs/RUNBOOK.md.
FROM postgres:16-alpine
RUN apk add --no-cache aws-cli age
COPY scripts/backup.sh /usr/local/bin/backup.sh
USER postgres
ENTRYPOINT ["/usr/local/bin/backup.sh"]
