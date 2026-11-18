# syntax=docker/dockerfile:1

# --- dev: hot reload (air) + debugger (delve); source is bind-mounted ---
FROM golang:{{ cookiecutter.go_version }}-alpine AS dev
RUN --mount=type=cache,target=/root/.cache/go-build --mount=type=cache,target=/go/pkg/mod \
    go install github.com/air-verse/air@v1.67.4 && \
    go install github.com/go-delve/delve/cmd/dlv@v1.27.2
WORKDIR /app
EXPOSE 8080 2345
CMD ["air", "-c", ".air.toml"]

# --- build: static binary ---
FROM golang:{{ cookiecutter.go_version }}-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/{{ cookiecutter.project_slug }} ./cmd/{{ cookiecutter.project_slug }}

# --- runtime (default target) ---
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/{{ cookiecutter.project_slug }} /{{ cookiecutter.project_slug }}
COPY configs/config.yaml /configs/config.yaml
ENV {{ cookiecutter.env_prefix }}_CONFIG=/configs/config.yaml \
    {{ cookiecutter.env_prefix }}_ENV=prod \
    {{ cookiecutter.env_prefix }}_LOG_LEVEL=info \
    {{ cookiecutter.env_prefix }}_LOG_FORMAT=json
{%- if cookiecutter.use_postgres == 'y' %} \
    {{ cookiecutter.env_prefix }}_DB_AUTO_MIGRATE=false
{%- endif %}
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/{{ cookiecutter.project_slug }}"]
