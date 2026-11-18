module {{ cookiecutter.module_path }}/deploy

go {{ cookiecutter.go_version }}

require (
	github.com/go-playground/validator/v10 v10.30.5
	github.com/pulumi/pulumi-gcp/sdk/v9 v9.37.1
	github.com/pulumi/pulumi/sdk/v3 v3.272.0
	github.com/spf13/viper v1.21.0
	github.com/stretchr/testify v1.12.1
)
