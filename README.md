# go-cloud-run-template

Cookiecutter template for Go services on Cloud Run, distilled from blobster and gotifier.

| Area | Choice |
|---|---|
| HTTP | gin, request id, JSON access logs, JSON error envelope, `/healthz` + `/readyz` |
| Config | viper: `configs/config.yaml` + `<PREFIX>_<KEY>` env overrides, go-playground/validator |
| Data | optional Postgres: pgx pool (functional options, query/connect timeouts, `InTx`), sqlc with the DBTX as argument, goose migrations embedded in the binary (advisory lock, dedicated schema); optional Redis (go-redis) |
| Probes | `/healthz` liveness, `/readyz` list of dependency checks (`health.Checker` registry) |
| Wiring | samber/do; constructors use functional options (`logger.New`, `postgres.New`, `redis.New`, `http.NewServer`) |
| Logs | slog JSON with Cloud Logging keys (`severity`, `message`) |
| Tooling | mise (pinned tools), Taskfile, golangci-lint v2, pre-commit (Conventional Commits) |
| CI/CD | GitHub Actions: `ci` (lint, tests with testcontainers, docker build), release-please, `image` (Artifact Registry via WIF) |
| Deploy | Pulumi Go program in `deploy/`: Cloud Run min 0 instances, IAM-only access (`roles/run.invoker` for listed service accounts), Secret Manager |
| Agents | `AGENTS.md`, `CLAUDE.md` and a `<slug>-dev` skill for Claude Code (`.claude/skills`) and opencode (`.opencode/skills`) |

## Use

Requirements: `cookiecutter`, `mise`, `git`, `docker`; with GitHub/WIF setup also `gh` (logged in)
and `gcloud` (permissions on the WIF pool and the CI service account).

```sh
cookiecutter gh:llorenzinho/go-cloud-run-template      # or: cookiecutter /path/to/this/repo
```

After rendering, `hooks/post_gen_project.py`:

1. removes the code of the options left out (Postgres, the `items` example, Redis) and links
   the skill for opencode;
2. `mise install`, `sqlc generate` (with Postgres), `go mod tidy` (server + deploy), `golangci-lint fmt`;
3. `git init` + first commit `feat: initial commit …` on `main`;
4. `setup_wif=y`: creates the OIDC provider `github-<slug>` in `wif_pool`, restricted to
   `assertion.repository == '<owner>/<slug>'`, and grants `roles/iam.workloadIdentityUser`
   on `wif_service_account` (skipped when the whole pool is already granted);
5. `create_github_repo=y`: `gh repo create`, repository variables `GCP_PROJECT`, `GCP_REGION`,
   `AR_REPOSITORY`, `IMAGE_NAME`, `WIF_PROVIDER`, `WIF_SERVICE_ACCOUNT`, lets Actions open PRs,
   pushes `main`.

release-please then opens the `v0.1.0` release PR; merging it tags the release and the
`image` workflow pushes `<region>-docker.pkg.dev/<project>/<ar_repository>/<slug>:v0.1.0`.
Deploy with `task deploy:up STACK=dev` (see the generated README).

If a GitHub/GCP step fails the generated project is kept; the error says which command
failed so it can be rerun by hand.

The CI service account must be able to push to the Artifact Registry repository
(`roles/artifactregistry.writer`); the template does not grant it.

## Scaffold skill (Claude Code / opencode)

`plugins/go-cloud-run/skills/go-cloud-run-scaffold` teaches an agent to collect the
answers, check prerequisites, confirm the GitHub/GCP actions and run cookiecutter.

```sh
# Claude Code, as a plugin from this repository's marketplace
/plugin marketplace add llorenzinho/go-cloud-run-template
/plugin install go-cloud-run@go-cloud-run-template

# or symlink it from a local checkout (Claude Code + opencode)
task skill:install
export GO_CLOUD_RUN_TEMPLATE=$PWD    # scaffold from the checkout instead of GitHub
```

## Options (`cookiecutter.json`)

| Key | Default | |
|---|---|---|
| `project_name` | `My Service` | |
| `project_slug` | from name | repo, binary, Cloud Run service; 3-22 chars |
| `github_owner` | required | GitHub user or org of the new repository |
| `module_path` | `github.com/<owner>/<slug>` | |
| `env_prefix` | slug upper case | config env vars |
| `base_path` | `/<slug>` | API under `<base_path>/v1` |
| `use_postgres` | `y` | pgx pool + sqlc + goose, compose service, `db:*`/`migrate:*`/`sqlc` tasks, deploy secret |
| `db_schema` | slug with `_` | Postgres schema of the service (fixed: written in the migrations) |
| `include_examples` | `y` | deletable `items` CRUD (migration, queries, seed, repository with `InTx`, service, handler, tests); only with Postgres |
| `use_redis` | `n` | |
| `gcp_project`, `ar_repository` | required | project of Artifact Registry and Cloud Run, Docker repository |
| `gcp_region` | `europe-west1` | |
| `wif_pool`, `wif_service_account` | required | Workload Identity pool, service account impersonated by Actions |
| `repo_visibility` | `private` | |
| `create_github_repo`, `setup_wif` | `y` | `n` to only render locally |

## Personal defaults

The account values have no default in `cookiecutter.json`. Keep yours in
`~/.cookiecutterrc` so cookiecutter pre-fills them (and `--no-input` works):

```yaml
default_context:
  github_owner: my-user
  gcp_project: my-project
  ar_repository: my-repo
  wif_pool: github
  wif_service_account: ci@my-project.iam.gserviceaccount.com
```

Or pass them on the command line: `cookiecutter … gcp_project=my-project …`.

## Developing the template

```sh
task bake V=yyn            # render use_postgres/include_examples/use_redis = y/y/n into .bake/demo-yyn
task verify:one V=ynn      # bake one variant: build, lint, unit tests, docker build, sqlc check, deploy check
task verify                # every variant in VARIANTS (yyn ynn nnn nny yyy)
task verify:integration    # testcontainers integration tests of the baked variants
```

`image.yml` and `release-please.yml` are copied without rendering; `ci.yml` and the
Taskfile wrap their `${{ }}` / `{{.VAR}}` parts in `{% raw %}`. Derived flag:
`cookiecutter.__examples` (examples and Postgres both enabled).
