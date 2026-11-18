// Command devtoken creates the local dev key pair and signs tokens with it.
// It is a development tool: the service image never contains it, nor a
// private key.
//
// Usage:
//
//	devtoken keygen [-dir .dev/auth] [-alg RS256|ES256|EdDSA] [-force]
//	devtoken token  [-key .dev/auth/private.pem] [-sub dev-user] [-email ...] [-roles admin,viewer] [-ttl 1h]
//
// token reads issuer, audience and roles_claim from the service config
// (configs/config.yaml + {{ cookiecutter.env_prefix }}_* env), so the token matches what the server
// verifies in static mode with .dev/auth/public.pem.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"{{ cookiecutter.module_path }}/internal/auth/authtest"
	"{{ cookiecutter.module_path }}/internal/config"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "devtoken:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: devtoken keygen|token [flags]")
	}
	switch args[0] {
	case "keygen":
		return keygen(args[1:])
	case "token":
		return token(args[1:])
	default:
		return fmt.Errorf("unknown command %q (keygen, token)", args[0])
	}
}

func keygen(args []string) error {
	fl := flag.NewFlagSet("keygen", flag.ContinueOnError)
	dir := fl.String("dir", ".dev/auth", "output directory")
	alg := fl.String("alg", "RS256", "key algorithm: RS256, ES256 or EdDSA (must be in auth.algorithms)")
	force := fl.Bool("force", false, "overwrite an existing key pair")
	if err := fl.Parse(args); err != nil {
		return err
	}
	priv := filepath.Join(*dir, "private.pem")
	pub := filepath.Join(*dir, "public.pem")
	if _, err := os.Stat(priv); err == nil && !*force {
		fmt.Println("key pair already in", *dir, "(use -force to replace it)")
		return nil
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}

	iss, err := authtest.NewIssuer(*alg)
	if err != nil {
		return err
	}
	privPEM, err := iss.PrivateKeyPEM()
	if err != nil {
		return err
	}
	pubPEM, err := iss.PublicKeyPEM()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(*dir, 0o750); err != nil {
		return err
	}
	if err := os.WriteFile(priv, privPEM, 0o600); err != nil {
		return err
	}
	// Readable by the nonroot user of the image (docker compose app-prod).
	if err := os.WriteFile(pub, pubPEM, 0o644); err != nil { //nolint:gosec // public key
		return err
	}
	fmt.Printf("wrote %s and %s (%s)\n", priv, pub, *alg)
	return nil
}

func token(args []string) error {
	fl := flag.NewFlagSet("token", flag.ContinueOnError)
	key := fl.String("key", ".dev/auth/private.pem", "private key written by keygen")
	sub := fl.String("sub", "dev-user", "subject (sub)")
	email := fl.String("email", "", "email claim")
	roles := fl.String("roles", "", "comma-separated roles, written at auth.roles_claim")
	ttl := fl.Duration("ttl", time.Hour, "validity")
	if err := fl.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load("")
	if err != nil {
		return err
	}
	data, err := os.ReadFile(*key)
	if err != nil {
		return fmt.Errorf("%w (run: task auth:keygen)", err)
	}
	iss, err := authtest.LoadIssuer(data)
	if err != nil {
		return err
	}
	if len(cfg.Auth.Audiences) == 0 {
		return errors.New("auth.audiences is empty")
	}
	iss.Issuer = cfg.Auth.Issuer
	iss.Audience = cfg.Auth.Audiences[0]
	iss.RolesClaim = cfg.Auth.RolesClaim

	c := authtest.Claims{Subject: *sub, Email: *email, TTL: *ttl}
	if *roles != "" {
		c.Roles = strings.Split(*roles, ",")
	}
	s, err := iss.Token(c)
	if err != nil {
		return err
	}
	fmt.Println(s)
	return nil
}
