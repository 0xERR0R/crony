![Build](https://github.com/0xERR0R/crony/workflows/Build/badge.svg)

# crony

Simple docker cron job scheduler.

## Features

- Start existing docker container periodically
- Send a mail after the job execution (always, or only on failure) with stdout/stderr
- Automatically register new containers
- healthchecks.io integration (job start/return code, duration)

## Usage

Start crony with following `docker-compose.yml`

```yaml
version: "2.1"
services:
  crony:
    image: ghcr.io/0xerr0r/crony:latest
    container_name: crony
    volumes:
      # needs access to host docker socket to read container labels and start job containers
      - /var/run/docker.sock:/var/run/docker.sock:ro
      # to synchronize the time zone with host
      - /etc/localtime:/etc/localtime:ro
    environment:
      # mail settings
      - SMTP_HOST=smtp.gmail.com 
      - SMTP_PORT=587
      - SMTP_USER=xxx@googlemail.com
      - SMTP_PASSWORD=xxx
      - MAIL_TO=xxx@gmail.com
      - MAIL_FROM=CRONY <xxx@googlemail.com>
      # global mail policy: always, never or onerror
      - MAIL_POLICY=always
      # optional: log level (one of trace, debug, info, warn, error, fatal), info is default if not set
      - LOG_LEVEL=info
    mem_limit: 30MB
```


Use following labels in your docker container which should be scheduled by crony:

```yaml
...
      labels:
          # cron string
          - crony.schedule="*/15 6-23 * * *"
          # optional to override the global settings
          - crony.mail_policy=onerror
          # optional, use following job UUID for reporting to healthcheck.io
          - crony.hcio_uuid=394ed711-afca-4a4f-9cdb-16b7e976418e
...
```