// Command deploy is the Pulumi program of {{ cookiecutter.project_name }}. The stack
// configuration is Pulumi config (Pulumi.<stack>.yaml, see internal/config).
package main

import (
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"{{ cookiecutter.module_path }}/deploy/internal/config"
	"{{ cookiecutter.module_path }}/deploy/internal/infra"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		cfg, sec, err := config.Load(ctx)
		if err != nil {
			return err
		}
		for _, w := range cfg.Warnings() {
			_ = ctx.Log.Warn(w, nil)
		}
		return infra.Deploy(ctx, cfg, sec)
	})
}
