# Recall

**Catches \*arr imports that score lower than their grab.**

[![ci](https://img.shields.io/github/actions/workflow/status/santiagosayshey/recall/ci.yml?branch=develop&label=ci&logo=githubactions&logoColor=white)](https://github.com/santiagosayshey/recall/actions/workflows/ci.yml)
[![release](https://img.shields.io/github/v/release/santiagosayshey/recall?label=release&logo=github&logoColor=white)](https://github.com/santiagosayshey/recall/releases)
[![license](https://img.shields.io/github/license/santiagosayshey/recall?logo=opensourceinitiative&logoColor=white)](LICENSE)

## Overview

Radarr and Sonarr score a release twice: once at grab, against the indexer's title, and once at import, against the torrent's own name. When the two names differ the file can land at a far lower score than it was chosen at, and the next search replaces it with something worse. Neither app notices.

Recall receives both apps' webhooks, keeps the grab and the import for each download, and logs a decision for every import. When the import comes in lower the line names both titles, both scores, and the formats that were lost, so a log watcher can page on it. See [docs/design.md](docs/design.md).

## Getting started

Recall runs as one container. A config file lists your Radarr and Sonarr instances. On start, Recall adds a webhook connection to each one that points back at itself. The connection sends a secret header with every event, and Recall checks it.

### Requirements

- Docker with Compose.
- Each instance's API key, from Settings, General.
- Each instance's name, from the same page or the `RADARR__APP__INSTANCENAME` and `SONARR__APP__INSTANCENAME` environment variables. Recall keys events on it, so two instances must not share one.

### Compose

```yaml
services:
  recall:
    image: ghcr.io/santiagosayshey/recall:0.1.0
    container_name: recall
    restart: unless-stopped
    environment:
      TZ: Australia/Adelaide
      RECALL_SECRET: ${RECALL_SECRET}
      RADARR_API_KEY: ${RADARR_API_KEY}
      SONARR_API_KEY: ${SONARR_API_KEY}
    volumes:
      - ./config/config.yml:/config/config.yml:ro
      - ./appdata:/data
    networks:
      - media   # wherever the *arr containers are
```

Recall must be reachable from the apps at the URL in the config, and it must reach each app at that app's URL. `/data` must be writable by the user the container runs as, which is `nonroot` (65532) unless `user:` says otherwise; a fresh named volume is set up for that, a bind mount needs the right owner.

### Configuration

`config.yml`, mounted at `/config/config.yml`. Values may reference environment variables as `${NAME}`, so the file can be tracked and the keys kept out of it.

```yaml
# Where the apps post. Recall listens on 8471; the host is whatever the
# apps resolve the Recall container as.
url: http://recall:8471/webhook

# Sent by the apps as the X-Recall-Secret header on every event and checked
# by Recall. Anything without it is refused. Pick a long random string.
secret: ${RECALL_SECRET}

instances:
  - name: Radarr          # must equal the app's instance name
    app: radarr           # radarr or sonarr
    url: http://radarr:7878
    apiKey: ${RADARR_API_KEY}
  - name: Sonarr
    app: sonarr
    url: http://sonarr:8989
    apiKey: ${SONARR_API_KEY}
```

On start Recall checks each app calls itself by the configured name, then creates a connection named Recall, or updates it if it differs, or leaves it alone if it already matches. It reads the connection back and refuses to consider the instance registered unless it is exactly what was written. An app that is not up yet is retried, so the order containers start in does not matter. The connection asks for grabs, imports and upgrades only.

`recall register` does the same once and exits non-zero on any failure, for checking a config by hand.

### Environment

| Variable | Default | Meaning |
| --- | --- | --- |
| `RECALL_CONFIG` | `/config/config.yml` | The config file. Without one at the default path Recall only receives, with no secret and no registration. |
| `RECALL_DATA` | `/data` | Where the files go. |
| `RECALL_LISTEN` | `:8471` | Address to serve on. |

### Files

Three append-only files of JSON Lines under `/data`: `grabs.jsonl`, `imports.jsonl`, `decisions.jsonl`. Each line is the normalised record, and for grabs and imports the raw webhook body under `raw`, so the files are the archive and read naturally with `jq`.

### Log

Every decision is one line on stdout, result first:

```
level=INFO msg=decision result=drift instance=Radarr media="100% Wolf (2020)" release="100 Percent Wolf 2020 1080p BluRay DD5.1 x264-PTer" file="100 Percent Wolf.2020.1080p.BluRay.DD5.1.x264- PTer" grabScore=881400 importScore=-299599 delta=-1180999 lost="[1080p Quality Tier 5]" gained="[Release Group (Missing)]" path="/media/library/movies/100% Wolf (2020)/100% Wolf (2020).mkv" download=…
```

Match `result=drift` with whatever watches your container logs. Recall never calls out itself.

## Development

Go 1.26 and Docker. `make check` runs everything CI runs: the unit checks, then the image built and the captured webhooks replayed through it beside a real Radarr and Sonarr.

```bash
make check-go            # gofmt, vet, staticcheck, our analyzers, unit tests
make check-integration   # build the image, run test/ against it
make run                 # serve locally on 8471 with no config
```
