---
name: go-cloud-run-scaffold
description: Use when the user wants to create, scaffold or bootstrap a new Go service / microservice / API for Cloud Run, or mentions go-cloud-run-template. Generates the project with cookiecutter (gin, optional Postgres with pgx pool + sqlc + goose, optional Redis, viper, samber/do, mise, Taskfile, GitHub Actions, Pulumi deploy) and can create the GitHub repository, its variables and the Workload Identity provider.
---

# Scaffold a Go Cloud Run service

The template is `go-cloud-run-template` (cookiecutter). Source, first match wins:

1. `$GO_CLOUD_RUN_TEMPLATE` (local path or `gh:owner/repo`)
2. `gh:llorenzinho/go-cloud-run-template`

## 1. Collect the answers

Ask only for what the user has not said; defaults in brackets. The account values
(**required**, no default) may already be in `~/.cookiecutterrc` under `default_context`:
read it first and only ask for what is missing.

| Key | Meaning |
|---|---|
| `project_name` | Human name, e.g. "Orders". Required. |
| `project_slug` | [name lower-kebab] repo, binary, Cloud Run service. 3-22 chars `[a-z0-9-]`. |
| `description` | One line for the GitHub repo. |
| `use_postgres` | [`y`] pgx pool + sqlc + goose migrations, Postgres compose service, `db:*`/`migrate:*` tasks, deploy secret `db_password`. `n` = no database code at all. |
| `db_schema` | [slug with `_`] schema holding the tables; fixed (written in the migrations). |
| `include_examples` | [`y`] deletable `items` CRUD showing every layer; ignored without Postgres. |
| `use_redis` | [`n`] `y` adds go-redis, a Redis compose service, readiness check and deploy secret. |
| `base_path` | [`/<slug>`] API under `<base_path>/v1`. |
| `github_owner` | **required**: GitHub user or org of the new repository. |
| `repo_visibility` | [`private`] or `public` |
| `gcp_project`, `ar_repository` | **required**: GCP project, Artifact Registry Docker repository. |
| `gcp_region` | [`europe-west1`] |
| `wif_pool`, `wif_service_account` | **required**: Workload Identity pool id, service account impersonated by GitHub Actions (`…@<project>.iam.gserviceaccount.com`). |
| `create_github_repo`, `setup_wif` | [`y`, `y`] `n` renders locally only |

## 2. Check prerequisites

Run these and stop with a clear message if one fails:

```sh
command -v cookiecutter mise git docker
gh auth status                 # only if create_github_repo=y
gcloud auth list --filter=status:ACTIVE --format='value(account)'   # only if create_github_repo=y or setup_wif=y
gh repo view <owner>/<slug>    # must FAIL: the repository must not exist yet
test ! -e <output dir>/<slug>  # the target directory must not exist
```

## 3. Confirm before anything outward-facing

With `create_github_repo=y` or `setup_wif=y`, show the user exactly what will happen and wait
for an explicit yes:

- GitHub repository `<owner>/<slug>` (<visibility>) created, 6 Actions variables set,
  Actions allowed to open PRs, `main` pushed;
- WIF provider `projects/<number>/locations/global/workloadIdentityPools/<pool>/providers/github-<slug>`
  accepting only `<owner>/<slug>`, and `roles/iam.workloadIdentityUser` on `<wif_service_account>`
  for it (skipped if the pool already has it).

If the user only wants to look at the code, use `create_github_repo=n setup_wif=n`.

## 4. Generate

Run from the directory that should contain the project (default: the current one):

```sh
cookiecutter --no-input "${GO_CLOUD_RUN_TEMPLATE:-gh:llorenzinho/go-cloud-run-template}" \
  project_name="Orders" project_slug=orders use_postgres=y include_examples=y use_redis=n \
  description="Orders service" repo_visibility=private \
  github_owner=my-user gcp_project=my-project ar_repository=my-repo \
  wif_pool=github wif_service_account=ci@my-project.iam.gserviceaccount.com \
  create_github_repo=y setup_wif=y
```

Pass every collected answer as `key=value` (values in `~/.cookiecutterrc` can be
omitted); quote values with spaces. The post-generation
hook installs the mise tools, generates sqlc code, runs `go mod tidy`, makes the first commit,
then does the GitHub/WIF steps. It takes a few minutes the first time (module downloads).

If a GitHub/GCP step fails the project is kept and the hook prints the failed command:
fix the cause (auth, permissions, existing repo) and rerun only that command; never rerun
cookiecutter over the existing directory.

## 5. Verify and report

```sh
cd <slug> && task build lint test
gh variable list --repo <owner>/<slug>       # if the repo was created
gh pr list --repo <owner>/<slug>             # release-please PR appears within a minute
```

Tell the user, briefly:

- where the project is and the repository URL;
- merging the release-please PR tags `v0.1.0` and the `image` workflow pushes
  `<region>-docker.pkg.dev/<project>/<ar_repository>/<slug>:v0.1.0`;
- the CI service account needs `roles/artifactregistry.writer` on the repository (not granted
  by the template);
- local database (with Postgres): `task db:seed` migrates and loads the example data;
  other databases: `task migrate:up ENV=<env>` with `.env.<env>` (from `.env.prod.example`);
- to deploy: edit `deploy/config/dev/config.yaml` (`access.callers`, `app.db` host/user/name),
  copy `deploy/config/dev/secret.example.yaml` to `secret.yaml` (`db_password`), then
  `task deploy:check deploy:up STACK=dev`;
- with examples: the README lists the files to delete to drop `items`;
- the project has its own `<slug>-dev` skill and `AGENTS.md` for further work.
