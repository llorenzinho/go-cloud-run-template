module {{ cookiecutter.module_path }}

go {{ cookiecutter.go_version }}

require (
	github.com/gin-gonic/gin v1.12.0
	github.com/go-playground/validator/v10 v10.30.5
{%- if cookiecutter.use_postgres == 'y' %}
	github.com/jackc/pgx/v5 v5.11.0
	github.com/pressly/goose/v3 v3.29.0
{%- endif %}
{%- if cookiecutter.use_redis == 'y' %}
	github.com/redis/go-redis/v9 v9.23.0
	github.com/testcontainers/testcontainers-go/modules/redis v0.44.0
{%- endif %}
	github.com/samber/do/v2 v2.1.0
	github.com/spf13/viper v1.21.0
	github.com/stretchr/testify v1.12.1
{%- if cookiecutter.use_postgres == 'y' or cookiecutter.use_redis == 'y' %}
	github.com/testcontainers/testcontainers-go v0.44.0
{%- endif %}
{%- if cookiecutter.use_postgres == 'y' %}
	github.com/testcontainers/testcontainers-go/modules/postgres v0.44.0
{%- endif %}
)
