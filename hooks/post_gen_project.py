"""Finishes the generated project.

1. Removes the files of the options left out (Postgres, examples, Redis, auth).
2. Installs the tools pinned in mise.toml, resolves Go modules, generates sqlc
   code and formats everything.
3. Creates the first commit on main.
4. create_github_repo=y: creates the GitHub repository, sets the repository
   variables read by the image workflow and lets Actions open PRs (release-please).
5. setup_wif=y: creates the Workload Identity provider github-<slug> restricted
   to the repository and lets it impersonate the CI service account.
6. Pushes main: release-please opens the release PR; merging it tags v0.1.0 and
   builds the image into Artifact Registry.

Set COOKIECUTTER_SKIP_TOOLS=1 to skip step 2 (template tests that only render).
"""

import json
import os
import shutil
import subprocess
import sys
from pathlib import Path

SLUG = "{{ cookiecutter.project_slug }}"
OWNER = "{{ cookiecutter.github_owner }}"
REPO = f"{OWNER}/{SLUG}"
DESCRIPTION = "{{ cookiecutter.description }}"
VISIBILITY = "{{ cookiecutter.repo_visibility }}"
USE_POSTGRES = "{{ cookiecutter.use_postgres }}" == "y"
EXAMPLES = "{{ cookiecutter.__examples }}" == "y"
USE_REDIS = "{{ cookiecutter.use_redis }}" == "y"
USE_AUTH = "{{ cookiecutter.use_auth }}" == "y"
AUTH_EXAMPLES = "{{ cookiecutter.__auth_examples }}" == "y"
CREATE_REPO = "{{ cookiecutter.create_github_repo }}" == "y"
SETUP_WIF = "{{ cookiecutter.setup_wif }}" == "y"
GCP_PROJECT = "{{ cookiecutter.gcp_project }}"
GCP_REGION = "{{ cookiecutter.gcp_region }}"
AR_REPOSITORY = "{{ cookiecutter.ar_repository }}"
WIF_POOL = "{{ cookiecutter.wif_pool }}"
WIF_SA = "{{ cookiecutter.wif_service_account }}"
WIF_PROVIDER_ID = f"github-{SLUG}"

ROOT = Path.cwd()
MODULES = [ROOT, ROOT / "deploy"]

POSTGRES_PATHS = [
    "internal/postgres",
    "internal/db",
    "db",
    "sqlc.yaml",
    ".env.prod.example",
    "test/integration/postgres_test.go",
]

EXAMPLE_PATHS = [
    "db/migrations/00002_create_items.sql",
    "db/queries/items.sql",
    "db/seed/00001_items.sql",
    "internal/domain/item.go",
    "internal/repository",
    "internal/service",
    "internal/http/handler/items.go",
    "internal/http/items_test.go",
    "test/integration/items_test.go",
    "api/http/items.http",
]

REDIS_PATHS = [
    "internal/redis",
]

AUTH_PATHS = [
    "internal/auth",
    "internal/http/middleware/auth.go",
    "internal/http/middleware/auth_test.go",
    "internal/http/handler/me.go",
    "internal/http/auth_test.go",
    "cmd/devtoken",
    "api/http/auth.http",
]

# The public route example (<base>/v1/public/ping) comes with the items example.
AUTH_EXAMPLE_PATHS = [
    "internal/http/handler/public.go",
]

# Integration tests only exist when there is a dependency to start.
INTEGRATION_PATHS = [
    "internal/testutil",
    "test",
]


def step(msg: str) -> None:
    print(f"\n\033[1;34m==> {msg}\033[0m", flush=True)


def run(*cmd: str, cwd: Path = ROOT, capture: bool = False, check: bool = True) -> subprocess.CompletedProcess:
    print("   $ " + " ".join(cmd), flush=True)
    return subprocess.run(cmd, cwd=cwd, check=check, text=True, capture_output=capture)


def mise(*cmd: str, cwd: Path = ROOT) -> None:
    run("mise", "exec", "--", *cmd, cwd=cwd)


def require(*tools: str) -> None:
    missing = [t for t in tools if shutil.which(t) is None]
    if missing:
        sys.exit(f"ERROR: missing tools: {', '.join(missing)}")


def remove(reason: str, paths: list[str]) -> None:
    step(f"Removing {reason}")
    for p in paths:
        path = ROOT / p
        if path.is_dir():
            shutil.rmtree(path)
        elif path.exists():
            path.unlink()


def prune() -> None:
    if not USE_POSTGRES:
        remove("Postgres support (use_postgres=n)", POSTGRES_PATHS)
    if not EXAMPLES:
        remove("the items example (include_examples=n)", EXAMPLE_PATHS)
        if USE_POSTGRES:
            (ROOT / "db" / "seed").mkdir(parents=True, exist_ok=True)
            (ROOT / "db" / "seed" / ".gitkeep").touch()
    if not USE_REDIS:
        remove("Redis support (use_redis=n)", REDIS_PATHS)
    if not USE_AUTH:
        remove("JWT authentication (use_auth=n)", AUTH_PATHS)
    if not AUTH_EXAMPLES:
        remove("the public route example", AUTH_EXAMPLE_PATHS)
    api = ROOT / "api" / "http"
    if api.is_dir() and not any(api.glob("*.http")):
        shutil.rmtree(ROOT / "api")
    if not USE_POSTGRES and not USE_REDIS:
        remove("integration tests (no dependency)", INTEGRATION_PATHS)


def opencode_skill() -> None:
    # opencode reads skills from .opencode/skills: share the Claude Code skill.
    skills = ROOT / ".opencode" / "skills"
    skills.mkdir(parents=True, exist_ok=True)
    name = f"{SLUG}-dev"
    (skills / name).symlink_to(Path("..", "..", ".claude", "skills", name))


def toolchain() -> None:
    require("mise")
    step("Installing tools (mise)")
    run("mise", "trust", "--quiet", str(ROOT / "mise.toml"))
    run("mise", "install")

    step("Resolving Go modules, generating sqlc code, formatting")
    if USE_POSTGRES:
        mise("sqlc", "generate")
    for m in MODULES:
        mise("go", "mod", "tidy", cwd=m)
    mise("golangci-lint", "fmt")
    mise("golangci-lint", "fmt", "--config", str(ROOT / ".golangci.yml"), cwd=ROOT / "deploy")
    if USE_AUTH:
        step("Generating the local dev key pair (.dev/auth, git-ignored)")
        mise("go", "run", "./cmd/devtoken", "keygen")


def git_init() -> None:
    require("git")
    step("Creating the first commit")
    run("git", "init", "--quiet", "--initial-branch=main")
    run("git", "add", "--all")
    run("git", "commit", "--quiet", "--no-verify", "-m", "feat: initial commit from go-cloud-run-template")


def project_number() -> str:
    out = run(
        "gcloud", "projects", "describe", GCP_PROJECT, "--format=value(projectNumber)", capture=True
    ).stdout.strip()
    if not out:
        sys.exit(f"ERROR: cannot read the number of project {GCP_PROJECT}")
    return out


def github_repo(number: str) -> None:
    require("gh")
    step(f"Creating GitHub repository {REPO} ({VISIBILITY})")
    run("gh", "auth", "status", capture=True)
    exists = run("gh", "repo", "view", REPO, "--json", "name", capture=True, check=False).returncode == 0
    if exists:
        sys.exit(f"ERROR: {REPO} already exists on GitHub; nothing pushed. Remove it or pick another slug.")
    run(
        "gh", "repo", "create", REPO, f"--{VISIBILITY}",
        "--description", DESCRIPTION, "--source", str(ROOT), "--remote", "origin",
    )

    step("Setting repository variables (image workflow)")
    variables = {
        "GCP_PROJECT": GCP_PROJECT,
        "GCP_REGION": GCP_REGION,
        "AR_REPOSITORY": AR_REPOSITORY,
        "IMAGE_NAME": SLUG,
        "WIF_PROVIDER": f"projects/{number}/locations/global/workloadIdentityPools/{WIF_POOL}/providers/{WIF_PROVIDER_ID}",
        "WIF_SERVICE_ACCOUNT": WIF_SA,
    }
    for name, value in variables.items():
        run("gh", "variable", "set", name, "--repo", REPO, "--body", value)

    step("Allowing GitHub Actions to open pull requests (release-please)")
    run(
        "gh", "api", "--method", "PUT", f"repos/{REPO}/actions/permissions/workflow",
        "-f", "default_workflow_permissions=read",
        "-F", "can_approve_pull_request_reviews=true",
    )


def wif(number: str) -> None:
    require("gcloud")
    step(f"Workload Identity provider {WIF_POOL}/{WIF_PROVIDER_ID}")
    exists = run(
        "gcloud", "iam", "workload-identity-pools", "providers", "describe", WIF_PROVIDER_ID,
        f"--project={GCP_PROJECT}", "--location=global", f"--workload-identity-pool={WIF_POOL}",
        "--format=value(name)", capture=True, check=False,
    ).returncode == 0
    if exists:
        print("   provider already exists, left unchanged")
    else:
        run(
            "gcloud", "iam", "workload-identity-pools", "providers", "create-oidc", WIF_PROVIDER_ID,
            f"--project={GCP_PROJECT}", "--location=global", f"--workload-identity-pool={WIF_POOL}",
            f"--display-name={('GitHub ' + SLUG)[:32]}",
            "--issuer-uri=https://token.actions.githubusercontent.com",
            "--attribute-mapping=google.subject=assertion.sub,attribute.repository=assertion.repository",
            f"--attribute-condition=assertion.repository == '{REPO}'",
        )

    step(f"Letting {REPO} impersonate {WIF_SA}")
    pool = f"principalSet://iam.googleapis.com/projects/{number}/locations/global/workloadIdentityPools/{WIF_POOL}"
    member = f"{pool}/attribute.repository/{REPO}"
    policy = json.loads(run(
        "gcloud", "iam", "service-accounts", "get-iam-policy", WIF_SA,
        f"--project={GCP_PROJECT}", "--format=json", capture=True,
    ).stdout)
    granted = {
        m
        for b in policy.get("bindings", [])
        if b.get("role") == "roles/iam.workloadIdentityUser"
        for m in b.get("members", [])
    }
    if f"{pool}/*" in granted or member in granted:
        print("   already granted")
        return
    run(
        "gcloud", "iam", "service-accounts", "add-iam-policy-binding", WIF_SA,
        f"--project={GCP_PROJECT}", "--role=roles/iam.workloadIdentityUser", f"--member={member}",
        "--condition=None", "--format=none",
    )


def push() -> None:
    step("Pushing main")
    run("git", "push", "--quiet", "-u", "origin", "main")


def next_steps() -> None:
    image = f"{GCP_REGION}-docker.pkg.dev/{GCP_PROJECT}/{AR_REPOSITORY}/{SLUG}"
    print(f"""
\033[1;32mDone: {ROOT}\033[0m

Next steps:
  cd {SLUG} && task setup                      # tools, git hooks, .env
  task compose:up && task run                  # local dependencies and the server
""")
    if USE_AUTH:
        print("""  task auth:token SUB=alice ROLES=admin        # dev token signed with .dev/auth/private.pem
""")
    if USE_POSTGRES:
        print("""  task db:seed                                 # migrate + example data in the local Postgres
  task migrate:up ENV=<env>                    # other databases: copy .env.prod.example to .env.<env>
""")
    if CREATE_REPO:
        print(f"""  Release: merge the release-please PR on https://github.com/{REPO}/pulls
           -> tag v0.1.0 -> workflow "image" pushes {image}:v0.1.0
  Deploy:  edit deploy/config/dev/config.yaml (project, access.callers{', app.db' if USE_POSTGRES else ''}),
           cp deploy/config/dev/secret.example.yaml deploy/config/dev/secret.yaml
           task deploy:check deploy:up STACK=dev
""")


def main() -> None:
    prune()
    opencode_skill()
    if os.environ.get("COOKIECUTTER_SKIP_TOOLS") != "1":
        toolchain()
    git_init()
    if CREATE_REPO or SETUP_WIF:
        # A failure here must not make cookiecutter delete the generated
        # project: report it and keep the directory so the step can be redone.
        try:
            require("gcloud")
            number = project_number()
            if SETUP_WIF:
                wif(number)
            if CREATE_REPO:
                github_repo(number)
                push()
        except (subprocess.CalledProcessError, SystemExit) as e:
            if isinstance(e, subprocess.CalledProcessError):
                if e.stderr:
                    print(e.stderr, file=sys.stderr)
                e = f"command failed ({e.returncode}): {' '.join(e.cmd)}"
            print(f"\n\033[1;31mERROR: {e}\033[0m", file=sys.stderr)
            print(f"The project is kept in {ROOT}; finish GitHub/WIF setup by hand (see README).", file=sys.stderr)
            return
    next_steps()


if __name__ == "__main__":
    try:
        main()
    except subprocess.CalledProcessError as e:
        if e.stderr:
            print(e.stderr, file=sys.stderr)
        sys.exit(f"ERROR: command failed ({e.returncode}): {' '.join(e.cmd)}")
