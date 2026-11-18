// Package stacktest runs the Pulumi program in tests with the config of a
// committed stack file, without the Pulumi CLI.
package stacktest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"gopkg.in/yaml.v3"
)

// Project is the Pulumi project name, the namespace of the config keys.
const Project = "{{ cookiecutter.project_slug }}"

// Mocks records the inputs of every resource registered by the program.
type Mocks struct {
	mu        sync.Mutex
	Resources map[string]resource.PropertyMap // by type + "::" + name
}

// NewResource implements pulumi.MockResourceMonitor.
func (m *Mocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Resources[args.TypeToken+"::"+args.Name] = args.Inputs
	out := args.Inputs.Copy()
	switch args.TypeToken {
	case "gcp:serviceaccount/account:Account":
		email := args.Inputs["accountId"].StringValue() + "@prj.iam.gserviceaccount.com"
		out["email"] = resource.NewStringProperty(email)
		out["member"] = resource.NewStringProperty("serviceAccount:" + email)
	case "gcp:secretmanager/secretVersion:SecretVersion":
		out["version"] = resource.NewStringProperty("1")
	}
	return args.Name + "-id", out, nil
}

// Call implements pulumi.MockResourceMonitor.
func (m *Mocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	return args.Args, nil
}

// Run runs body as stack with the config of ../../Pulumi.<stack>.yaml, with
// set applied on top (keys without namespace; nil deletes) and secrets added
// as Pulumi secrets.
func Run(t *testing.T, stack string, set, secrets map[string]any, body pulumi.RunFunc) (*Mocks, error) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "Pulumi."+stack+".yaml")) //nolint:gosec // committed stack file, test only
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Config map[string]any `yaml:"config"`
	}
	if err := yaml.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	cfg := map[string]string{}
	put := func(key string, v any) {
		if v == nil {
			delete(cfg, key)
			return
		}
		if s, ok := v.(string); ok {
			cfg[key] = s
			return
		}
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		cfg[key] = string(raw)
	}
	for k, v := range file.Config {
		put(k, v)
	}
	for k, v := range set {
		put(Project+":"+k, v)
	}
	keys := []string{}
	for k, v := range secrets {
		put(Project+":"+k, v)
		keys = append(keys, Project+":"+k)
	}
	raw, _ := json.Marshal(cfg)
	t.Setenv(pulumi.EnvConfig, string(raw))
	rawKeys, _ := json.Marshal(keys)
	t.Setenv(pulumi.EnvConfigSecretKeys, string(rawKeys))

	m := &Mocks{Resources: map[string]resource.PropertyMap{}}
	return m, pulumi.RunErr(body, pulumi.WithMocks(Project, stack, m))
}
