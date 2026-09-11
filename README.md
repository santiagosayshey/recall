# Recall

**Catches *arr imports that score lower than their grab.**

[![ci](https://img.shields.io/github/actions/workflow/status/santiagosayshey/recall/ci.yml?branch=develop&label=ci&logo=githubactions&logoColor=white)](https://github.com/santiagosayshey/recall/actions/workflows/ci.yml)
[![release](https://img.shields.io/github/v/release/santiagosayshey/recall?label=release&logo=github&logoColor=white)](https://github.com/santiagosayshey/recall/releases)
[![license](https://img.shields.io/github/license/santiagosayshey/recall?logo=opensourceinitiative&logoColor=white)](LICENSE)

## Overview

Radarr and Sonarr score a release twice: once at grab, against the indexer's title, and once at import, against the torrent's own name. When the two names differ the file can land at a far lower score than it was chosen at, and the next search replaces it with something worse. Neither app notices.

Recall receives both apps' webhooks, keeps the grab and the import for each download, and logs a decision for every import. When the import comes in lower the line names both titles, both scores, and the formats that were lost. See [docs/design.md](docs/design.md).

## Getting started

Not yet. The first milestone is the repository itself.

## Development

Go 1.26. `make check` runs everything CI runs; `make run` serves on 8471.

```bash
make check
make run
```
