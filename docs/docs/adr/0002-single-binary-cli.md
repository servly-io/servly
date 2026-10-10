---
id: 0002-single-binary-cli
title: "0002: One binary, cobra and viper"
---

**Status:** accepted, 2026-10-10

## Context

MASTER_PLAN.md §4 suggests `cmd/server` and `cmd/migrate`, and allows adjusting the layout. Local operators install one file and one systemd unit. The plan also asks for environment-based configuration.

## Decision

* One binary, `servly`, built from `cmd/servly`, with commands `serve`, `migrate up|status`, `config path|view`, `version` and `options`. Later operator commands (`tenant create`, `backup`) join it.
* **cobra** for commands and flags.
* **viper** for configuration. Precedence: flag > `SERVLY_*` environment > YAML file > default. The file is optional. Lookup: `--config`, `$SERVLY_CONFIG`, `~/.config/servly-io/servly.yaml`, `/etc/servly-io/servly.yaml`.
* Unknown keys in the file are errors. Booleans and log levels are parsed strictly, not with viper's lenient casting.

## Alternatives

* Two binaries (`server`, `migrate`): two artifacts to ship and keep in step for no gain.
* Environment only, with no file and no flags: fine for containers, but awkward for a systemd install and for local development.

## Consequences

* Container deployments need only environment variables.
* Every new setting needs a key constant, a default in `config.defaults`, a flag where useful, and a row in [Local deployment](../local-deployment.md#configuration).
