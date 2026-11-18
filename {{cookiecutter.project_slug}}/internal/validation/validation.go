// Package validation wraps go-playground/validator with the service conventions:
// the "validate" struct tag, field names taken from json/mapstructure tags and
// errors wrapping domain.ErrInvalidInput with readable messages.
package validation

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"unicode"

	"github.com/go-playground/validator/v10"
	"github.com/go-playground/validator/v10/non-standard/validators"

	"{{ cookiecutter.module_path }}/internal/domain"
)

// New returns a validator configured with the service conventions. Packages
// that need struct-level rules create their own instance and register them on it.
func New() *validator.Validate {
	v := validator.New(validator.WithRequiredStructEnabled())
	v.SetTagName("validate")
	v.RegisterTagNameFunc(fieldName)
	for tag, fn := range map[string]validator.Func{
		"notblank": validators.NotBlank,
		"basepath": isBasePath,
	} {
		if err := v.RegisterValidation(tag, fn); err != nil {
			panic(err) // unreachable: static registration
		}
	}
	return v
}

var std = sync.OnceValue(New)

// Default returns the shared validator instance.
func Default() *validator.Validate { return std() }

// Struct validates s with the shared validator.
func Struct(s any) error { return Check(std(), s) }

// Check validates s with v and converts validation failures into a single
// error wrapping domain.ErrInvalidInput.
func Check(v *validator.Validate, s any) error {
	err := v.Struct(s)
	if err == nil {
		return nil
	}
	var verrs validator.ValidationErrors
	if !errors.As(err, &verrs) {
		return fmt.Errorf("%w: %w", domain.ErrInvalidInput, err)
	}
	msgs := make([]string, 0, len(verrs))
	for _, fe := range verrs {
		msgs = append(msgs, fieldPath(fe)+": "+message(fe))
	}
	return fmt.Errorf("%w: %s", domain.ErrInvalidInput, strings.Join(msgs, "; "))
}

// fieldName prefers the json tag, then mapstructure, then the snake_cased Go name.
func fieldName(f reflect.StructField) string {
	for _, tag := range []string{"json", "mapstructure"} {
		name, _, _ := strings.Cut(f.Tag.Get(tag), ",")
		switch name {
		case "-":
			return ""
		case "":
			continue
		default:
			return name
		}
	}
	return snake(f.Name)
}

// snake converts a Go identifier to snake_case: ContentType -> content_type.
func snake(s string) string {
	var b strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) {
			if i > 0 && !unicode.IsUpper(rune(s[i-1])) {
				b.WriteByte('_')
			}
			r = unicode.ToLower(r)
		}
		b.WriteRune(r)
	}
	return b.String()
}

// fieldPath drops the root struct name: "Config.log.level" -> "log.level".
func fieldPath(fe validator.FieldError) string {
	ns := fe.Namespace()
	if _, rest, ok := strings.Cut(ns, "."); ok {
		return rest
	}
	return ns
}

func message(fe validator.FieldError) string {
	p := fe.Param()
	switch fe.Tag() {
	case "required", "notblank":
		return "is required"
	case "oneof":
		return "must be one of [" + p + "]"
	case "min":
		return sizeMsg(fe, "at least", p)
	case "max":
		return sizeMsg(fe, "at most", p)
	case "gt":
		return "must be greater than " + p
	case "gte":
		return "must be greater than or equal to " + p
	case "lte":
		return "must be less than or equal to " + p
	case "ltefield":
		return "must be less than or equal to " + snake(p)
	case "url":
		return "must be a valid URL"
	case "cidr|ip", "ip|cidr":
		return "must be an IP address or CIDR"
	case "uuid", "uuid4", "uuid7":
		return "must be a valid UUID"
	case "basepath":
		return `must be a path like "/orders" (leading "/", no trailing "/")`
	default:
		if p != "" {
			return fmt.Sprintf("failed %q (%s)", fe.Tag(), p)
		}
		return fmt.Sprintf("failed %q", fe.Tag())
	}
}

func sizeMsg(fe validator.FieldError, op, p string) string {
	switch fe.Kind() {
	case reflect.String:
		return fmt.Sprintf("must be %s %s characters long", op, p)
	case reflect.Slice, reflect.Array, reflect.Map:
		return fmt.Sprintf("must contain %s %s items", op, p)
	default:
		return fmt.Sprintf("must be %s %s", op, p)
	}
}

// isBasePath validates an URL path prefix: "/seg[/seg...]" with segments of
// [A-Za-z0-9._-], no trailing slash.
func isBasePath(fl validator.FieldLevel) bool {
	p, ok := strings.CutPrefix(fl.Field().String(), "/")
	if !ok || p == "" {
		return false
	}
	for seg := range strings.SplitSeq(p, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return false
		}
		for _, r := range seg {
			ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-'
			if !ok {
				return false
			}
		}
	}
	return true
}

// Gin adapts the shared validator to gin's binding.StructValidator.
type Gin struct{}

// ValidateStruct validates structs, pointers to structs and slices of them.
func (Gin) ValidateStruct(obj any) error {
	if obj == nil {
		return nil
	}
	rv := reflect.ValueOf(obj)
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return nil
		}
		rv = rv.Elem()
	}
	switch rv.Kind() {
	case reflect.Struct:
		return Struct(rv.Interface())
	case reflect.Slice, reflect.Array:
		var errs []error
		for i := range rv.Len() {
			errs = append(errs, Gin{}.ValidateStruct(rv.Index(i).Interface()))
		}
		return errors.Join(errs...)
	default:
		return nil
	}
}

// Engine returns the underlying validator.
func (Gin) Engine() any { return std() }
