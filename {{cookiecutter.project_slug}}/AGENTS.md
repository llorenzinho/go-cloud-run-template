# {{ cookiecutter.project_name }}: agent instructions

Read `.claude/skills/{{ cookiecutter.project_slug }}-dev/SKILL.md` before changing code (also
available to opencode as `.opencode/skills/{{ cookiecutter.project_slug }}-dev`). Essentials:

- Run tools through mise: `mise exec -- go ...`, or the Taskfile (`task --list`).
{%- if cookiecutter.use_postgres == 'y' %}
- Never edit `internal/db/sqlc/`: change `db/queries` or `db/migrations` and run `task sqlc`.
- Queries take the DBTX: `db.DBTX()`, or the `tx` of `db.InTx` for atomic work.
{%- endif %}
- Two Go modules: the server (root) and `deploy/` (Pulumi). Run `task tidy`, `task lint`
  and `task test` for both.
{%- if cookiecutter.use_auth == 'y' %}
- Every route under `<base_path>/v*` needs a Bearer JWT (`internal/auth`, verified by
  `middleware.Authenticate`), except `<base_path>/v*/public/...`. Read the caller with
  `auth.FromContext`, restrict with `middleware.RequireRole`. Test with
  `internal/auth/authtest`, run locally with `task auth:token`.
{%- else %}
- No authentication code in the app: Cloud Run IAM (`roles/run.invoker`) is the only gate.
{%- endif %}
- Conventional Commits only (`feat:`, `fix:`, `chore:` …).
