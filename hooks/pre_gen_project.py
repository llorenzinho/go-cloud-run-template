"""Validates the cookiecutter answers before anything is generated."""

import re
import sys

SLUG = "{{ cookiecutter.project_slug }}"
MODULE = "{{ cookiecutter.module_path }}"
ENV_PREFIX = "{{ cookiecutter.env_prefix }}"
BASE_PATH = "{{ cookiecutter.base_path }}"
REGION = "{{ cookiecutter.gcp_region }}"
SA = "{{ cookiecutter.wif_service_account }}"
DB_SCHEMA = "{{ cookiecutter.db_schema }}"
# No defaults in cookiecutter.json: they identify an account. Put yours in
# ~/.cookiecutterrc (default_context) or pass them on the command line.
REQUIRED = {
    "github_owner": "{{ cookiecutter.github_owner }}",
    "gcp_project": "{{ cookiecutter.gcp_project }}",
    "ar_repository": "{{ cookiecutter.ar_repository }}",
    "wif_pool": "{{ cookiecutter.wif_pool }}",
    "wif_service_account": SA,
}
FORMATS = {
    "github_owner": r"[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})",
    "gcp_project": r"[a-z][a-z0-9-]{4,28}[a-z0-9]",
    "ar_repository": r"[a-z][a-z0-9-]{0,62}",
    "wif_pool": r"[a-z0-9-]{4,32}",
}
FLAGS = {
    "use_postgres": "{{ cookiecutter.use_postgres }}",
    "include_examples": "{{ cookiecutter.include_examples }}",
    "use_redis": "{{ cookiecutter.use_redis }}",
    "create_github_repo": "{{ cookiecutter.create_github_repo }}",
    "setup_wif": "{{ cookiecutter.setup_wif }}",
}

errors = []

missing = [k for k, v in REQUIRED.items() if not v.strip()]
for k in missing:
    errors.append(f"{k} is required (set it on the command line or in ~/.cookiecutterrc default_context)")
for k, pattern in FORMATS.items():
    if k not in missing and not re.fullmatch(pattern, REQUIRED[k]):
        errors.append(f"{k} {REQUIRED[k]!r} has an invalid format")

# Cloud Run service names allow 49 chars, but the slug also names the runtime
# service account (<slug>-dev-run, max 30) and the WIF provider (github-<slug>, max 32).
if not re.fullmatch(r"[a-z][a-z0-9-]{1,20}[a-z0-9]", SLUG):
    errors.append(
        f"project_slug {SLUG!r}: lowercase letters, digits and '-', start with a letter, 3-22 chars"
    )
if "github_owner" not in missing and not re.fullmatch(r"[a-z0-9.-]+(/[A-Za-z0-9._-]+)+", MODULE):
    errors.append(f"module_path {MODULE!r} is not a valid Go module path")
if not re.fullmatch(r"[A-Z][A-Z0-9_]*", ENV_PREFIX):
    errors.append(f"env_prefix {ENV_PREFIX!r}: upper case letters, digits and '_'")
if BASE_PATH and not re.fullmatch(r"(/[A-Za-z0-9._-]+)+", BASE_PATH):
    errors.append(f"base_path {BASE_PATH!r}: like /orders (leading '/', no trailing '/'), or empty")
if not re.fullmatch(r"[a-z_][a-z0-9_]{0,62}", DB_SCHEMA) or DB_SCHEMA in ("public", "pg_catalog") or DB_SCHEMA.startswith("pg_"):
    errors.append(f"db_schema {DB_SCHEMA!r}: lowercase letters, digits and '_', not public/pg_*")
if not re.fullmatch(r"[a-z]+-[a-z]+[0-9]", REGION):
    errors.append(f"gcp_region {REGION!r} is not a GCP region")
if "wif_service_account" not in missing and not re.fullmatch(r"[a-z0-9-]+@[a-z0-9-]+\.iam\.gserviceaccount\.com", SA):
    errors.append(f"wif_service_account {SA!r} is not a service account email")
for name, value in FLAGS.items():
    if value not in ("y", "n"):
        errors.append(f"{name} must be y or n, got {value!r}")

if errors:
    for e in errors:
        print(f"ERROR: {e}", file=sys.stderr)
    sys.exit(1)
