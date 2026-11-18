module {{ cookiecutter.module_path }}/deploy

go {{ cookiecutter.go_version }}

require (
	github.com/pulumi/pulumi-gcp/sdk/v9 v9.37.1
	github.com/pulumi/pulumi/sdk/v3 v3.272.0
	github.com/stretchr/testify v1.12.1
	gopkg.in/yaml.v3 v3.0.1
)
