// Command deploy is the Pulumi program of {{ cookiecutter.project_name }}. The stack
// configuration lives in config/ (see internal/config); `go run . -check -stack <name>`
// validates it without Pulumi.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"{{ cookiecutter.module_path }}/deploy/internal/config"
	"{{ cookiecutter.module_path }}/deploy/internal/infra"
)

func main() {
	check := flag.Bool("check", false, "validate the configuration of -stack and exit")
	stack := flag.String("stack", "", "stack to validate with -check")
	flag.Parse()

	if *check {
		if err := runCheck(*stack); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	pulumi.Run(func(ctx *pulumi.Context) error {
		cfg, sec, err := config.Load(config.Dir(), ctx.Stack())
		if err != nil {
			return err
		}
		for _, w := range cfg.Warnings() {
			_ = ctx.Log.Warn(w, nil)
		}
		return infra.Deploy(ctx, cfg, sec)
	})
}

func runCheck(stack string) error {
	if stack == "" {
		return fmt.Errorf("-check needs -stack")
	}
	cfg, _, err := config.Load(config.Dir(), stack)
	if err != nil {
		return err
	}
	for _, w := range cfg.Warnings() {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}
	fmt.Printf("stack %s: config and secrets valid (project %s, region %s, service %s, image %s)\n",
		stack, cfg.Project, cfg.Region, cfg.Service.Name, cfg.Image.Ref())
	return nil
}
