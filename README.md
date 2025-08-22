[![GitHub Workflow Status](https://img.shields.io/github/actions/workflow/status/0xERR0R/crony/build.yaml "Build")](https://github.com/0xERR0R/blocky/actions/workflows/build.yaml)
[![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/0xERR0R/blocky "Go version")](#)
[![Donation](https://img.shields.io/badge/buy%20me%20a%20coffee-donate-blueviolet.svg)](https://ko-fi.com/0xerr0r)

# crony

Crony is a simple, lightweight cron job scheduler for Docker containers. It monitors for containers with specific labels and starts them based on a cron schedule.

## Features

- **Cron-based Scheduling**: Start existing Docker containers periodically using cron expressions.
- **Email Notifications**: Send email reports with `stdout`/`stderr` after a job executes.
- **Flexible Mail Policies**: Configure email notifications to be sent always, only on failure, or never.
- **Automatic Registration**: Automatically detects and schedules new containers that have the required labels.
- **[Healthchecks.io](https://healthchecks.io) Integration**: Monitor your jobs using the popular Healthchecks.io service.

## Getting Started

The easiest way to run crony is by using the provided `docker-compose.yml` example.

```yaml
version: "2.1"
services:
  crony:
    image: ghcr.io/0xerr0r/crony:latest
    container_name: crony
    restart: unless-stopped
    volumes:
      # Access to the host Docker socket is required to read container labels and start job containers
      - /var/run/docker.sock:/var/run/docker.sock:ro
      # Synchronize the time zone with the host
      - /etc/localtime:/etc/localtime:ro
    environment:
      # See the Configuration section below for all available options
      - SMTP_HOST=smtp.example.com
      - SMTP_PORT=587
      - MAIL_TO=your-email@example.com
      - MAIL_FROM=Crony <crony@example.com>
      - MAIL_POLICY=onerror
      - LOG_LEVEL=info
```

## Configuration

Crony is configured using environment variables.

| Variable        | Description                                                                                                                                 | Required | Default |
|-----------------|---------------------------------------------------------------------------------------------------------------------------------------------|----------|---------|
| `SMTP_HOST`     | The hostname of your SMTP server.                                                                                                           | Yes      |         |
| `SMTP_PORT`     | The port of your SMTP server.                                                                                                               | Yes      |         |
| `MAIL_TO`       | The email address to send notification mails to.                                                                                            | Yes      |         |
| `MAIL_FROM`     | The "From" address to use in notification mails.                                                                                            | Yes      |         |
| `SMTP_USER`     | The username for your SMTP server. If provided, `SMTP_PASSWORD` must also be set. If omitted, crony will attempt to connect without auth.   | No       |         |
| `SMTP_PASSWORD` | The password for your SMTP server. Must be provided if `SMTP_USER` is set.                                                                  | No       |         |
| `MAIL_POLICY`   | The global policy for sending mail notifications. Can be overridden by a container label. See [Mail Policies](#mail-policies) for details.  | No       | `never` |
| `LOG_LEVEL`     | The logging level. One of `trace`, `debug`, `info`, `warn`, `error`, `fatal`.                                                               | No       | `info`  |

### Mail Policies

The `MAIL_POLICY` environment variable and the `crony.mail_policy` label accept the following values:

- `never`: Never send an email notification.
- `always`: Always send an email notification after the job runs.
- `onerror`: Only send an email notification if the job container exits with a non-zero status code.

## Container Labels

To have crony schedule one of your containers, you need to add specific labels to it.

| Label               | Description                                                                                             | Required | Example                               |
|---------------------|---------------------------------------------------------------------------------------------------------|----------|---------------------------------------|
| `crony.schedule`    | The cron expression that defines when the container should be started.                                  | Yes      | `*/15 6-23 * * *`                     |
| `crony.mail_policy` | Overrides the global `MAIL_POLICY` for this specific container. See [Mail Policies](#mail-policies).    | No       | `onerror`                             |
| `crony.hcio_uuid`   | The UUID for a [Healthchecks.io](https://healthchecks.io) check to monitor this job.                    | No       | `394ed711-afca-4a4f-9cdb-16b7e976418e` |

### Example Label Usage

Here is an example of how to add the required labels to a container in a `docker-compose.yml` file:

```yaml
services:
  my-backup-job:
    image: my-backup-image:latest
    labels:
      # Run every day at 3:00 AM
      - crony.schedule="0 3 * * *"
      # Only send a mail if the backup fails
      - crony.mail_policy=onerror
      # Ping Healthchecks.io
      - crony.hcio_uuid=394ed711-afca-4a4f-9cdb-16b7e976418e
```