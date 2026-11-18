---
name: {{ cookiecutter.project_slug }}-dev
description: Use when changing the {{ cookiecutter.project_name }} service in this repository - adding an endpoint{% if cookiecutter.use_postgres == 'y' %}, a table, a query, a migration or a transaction{% endif %}, a readiness check, a dependency or config key{% if cookiecutter.use_redis == 'y' %}, using Redis{% endif %}{% if cookiecutter.use_auth == 'y' %}, protecting a route or reading the caller (JWT auth, roles){% endif %}, writing tests, or touching the Dockerfile, GitHub workflows or the Pulumi deploy. Covers the layout, the conventions (samber/do + functional options, sqlc, goose, viper env overrides) and the commands to run before committing.
---

# Developing {{ cookiecutter.project_name }}

Go service on Cloud Run: gin{% if cookiecutter.use_postgres == 'y' %}, Postgres via pgx + sqlc, goose migrations{% endif %}{% if cookiecutter.use_redis == 'y' %}, Redis (go-redis){% endif %},
viper config, slog JSON logs, samber/do wiring. Tools are pinned in `mise.toml`;
run them as `mise exec -- <tool>` (Taskfile tasks already do).

## Layout

| Path | What |
|---|---|
| `cmd/{{ cookiecutter.project_slug }}/main.go` | Entry point: `serve` (default), `migrate up\|down\|status`, `version` |
| `configs/config.yaml` | Every config key with dev values; env `{{ cookiecutter.env_prefix }}_<KEY>` overrides (`.` → `_`) |
| `internal/config` | Config structs (`mapstructure` + `validate` tags) |
| `internal/di` | samber/do providers: config → functional options of each constructor |
| `internal/logger`{% if cookiecutter.use_postgres == 'y' %}, `internal/postgres`{% endif %}{% if cookiecutter.use_redis == 'y' %}, `internal/redis`{% endif %} | Constructors `New(..., opts ...Option)` |
| `internal/health` | `Checker` interface + `Registry` run by `/readyz` |
{%- if cookiecutter.use_auth == 'y' %}
| `internal/auth` | JWT `Verifier` (jwks / static key, options), `Principal`, `FromContext`, `Dev` |
| `internal/auth/authtest` | Signs tokens for tests and `cmd/devtoken` (never imported by the server) |
| `cmd/devtoken` | Dev key pair + token signing (`task auth:keygen`, `task auth:token`) |
{%- endif %}
| `internal/http` | Router (`NewRouter`), server (`NewServer(h, opts...)`), `middleware/`, `handler/` |
| `internal/domain` | Sentinel errors (`ErrNotFound`, `ErrConflict`, `ErrInvalidInput`) |
{%- if cookiecutter.__examples == 'y' %}
| `internal/service` | Business logic over the sqlc queries, transactions with `InTx`, pg errors → `domain.Err*` (example `items`) |
{%- endif %}
{%- if cookiecutter.use_postgres == 'y' %}
| `internal/postgres` | Pool (`New`, options), `DBTX()` with query timeout, `InTx`, `Migrate`, readiness `Check` |
| `db/migrations`, `db/queries`, `db/seed` | goose migrations (schema `{{ cookiecutter.db_schema }}`), sqlc queries per table, local seed |
| `internal/db/sqlc` | **Generated** by `task sqlc`: never edit |
{%- endif %}
| `test/integration` | `//go:build integration`, testcontainers |
| `deploy/` | Separate Go module: Pulumi program (Cloud Run, IAM, secrets) |

## Conventions

- **Functional options** for every infrastructure constructor: unexported `options`
  struct, `type Option func(*options)`, `WithX(...) Option`, defaults set in `New`
  before applying options. Add a `WithX` instead of a new positional parameter.
- **Wiring** only in `internal/di`: a `provideX(i do.Injector) (T, error)` registered
  with `do.Lazy` in `Infra` or `App`. Types needing cleanup implement `Shutdown()`
  (or `Shutdown(ctx) error`).
- **Readiness**: a dependency implements `health.Checker` (`Name() string`,
  `Check(ctx) error`) and is added with `health.WithChecker` in `provideHealth`.
  `/readyz` answers `{"status","checks":[{name,status,latency_ms,error}]}`, 503 if any is down.
- **Timeouts / context**: always pass the request `ctx` down. `http.request_timeout`
  sets its deadline (504 when exceeded){% if cookiecutter.use_postgres == 'y' %}; `db.timeouts.query` bounds each query
  whose ctx has no earlier deadline. Never use `context.Background()` in a request path{% endif %}.
- **Config**: add the field to the struct in `internal/config` with `mapstructure`
  and `validate` tags, add the key with a dev value and a comment to
  `configs/config.yaml`. If production needs a value, set it in
  `deploy/internal/infra.ServerEnv` and list it in `config.ManagedEnv` (deploy module).
- **Errors**: services return `domain.Err*`; handlers call `c.Error(err)` and
  return; `middleware.Errors` maps them to the JSON envelope
  `{"error":{"code","message"},"request_id"}`. Wrap with `%w`, never log and return.
- **Validation**: request structs use `validate` tags (go-playground) through
  `binding.Validator = validation.Gin{}`; messages come from `validation.message`.
- **Logs**: `*slog.Logger` from DI, `log.InfoContext(ctx, "msg", "key", v)`. JSON in
  production with `severity`/`message` keys for Cloud Logging. Never log secrets.
{%- if cookiecutter.use_auth == 'y' %}
- **Auth**: Cloud Run IAM admits the caller (`access.callers`, or allUsers with
  `access.public`); the app verifies the end-user Bearer JWT on every route of the
  `v1` group (`middleware.Authenticate`). Routes without a token go on the `public`
  group (`<base_path>/v1/public/...`). Read the caller with `auth.FromContext(ctx)`,
  restrict with `middleware.RequireRole("role")` (403). Never return the reason of a
  401 to the client. Tests: `authtest.NewIssuer("RS256")`, `issuer.MustToken(...)`,
  `issuer.Verifier(ctx)` (see `internal/http/auth_test.go`).
{%- else %}
- **Auth**: none in the app. Cloud Run only lets callers with `roles/run.invoker`
  through (`deploy/Pulumi.<stack>.yaml` → `access.callers`). Do not add JWT checks.
{%- endif %}
- **UUIDs**: stdlib `uuid` package (Go ≥ 1.27), `uuid.NewV7()` for new ids.
{%- if cookiecutter.use_postgres == 'y' %}
- **Queries**: sqlc methods take the DBTX (`emit_methods_with_db_argument`):
  `sqlc.New().GetX(ctx, db.DBTX(), ...)`. Several statements that must succeed together
  go in `db.InTx(ctx, pgx.TxOptions{}, func(tx sqlc.DBTX) error { ... })` and use `tx`
  for every call; return an error to roll back. Do not call `Pool` directly.
- **Schema**: everything lives in `{{ cookiecutter.db_schema }}` (the `search_path`): write unqualified
  names in migrations and queries.
{%- endif %}

## Recipes

{%- if cookiecutter.use_postgres == 'y' %}
**New table / query**
1. `task migrate:create -- add_orders` → edit `db/migrations/000NN_add_orders.sql`
   (`-- +goose Up` / `-- +goose Down`).
2. Add queries to `db/queries/<table>.sql` (`-- name: X :one|:many|:exec|:execrows`,
   `sqlc.narg` for optional params).
3. `task sqlc` → call the generated methods from a service (`internal/service`);
   return the sqlc row types, map pg errors to `domain.Err*`.
4. `task migrate:up` (local) and `task db:seed` if you add seed files.
   Other environments: `task migrate:up ENV=<name>` with `.env.<name>`.
{%- endif %}


**New endpoint**: handler in `internal/http/handler` with a `Register(r gin.IRoutes)`
method, service in `internal/service`, providers in `internal/di`, mount in
`NewRouter` under the `v1` group{% if cookiecutter.use_auth == 'y' %} (authenticated; `public` for open routes){% endif %}. Add a router test like `internal/http/server_test.go`
and an integration test in `test/integration`.

**New dependency**: `mise exec -- go get <module>@<version>`, then `task tidy`.

## Before committing

```sh
task fmt lint test          # gofumpt/goimports, golangci-lint, unit tests (server + deploy)
{%- if cookiecutter.use_postgres == 'y' %}
task sqlc:check             # generated code up to date
{%- endif %}
task test:integration       # needs Docker
```

Commits follow Conventional Commits (`feat:`, `fix:`, …), enforced by pre-commit:
release-please builds the changelog and the version from them. Merging the
release PR tags `vX.Y.Z` and the `image` workflow pushes the image to Artifact
Registry; `task deploy:up STACK=<stack>` deploys it.
