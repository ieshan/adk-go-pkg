package config

import (
	"fmt"
	"reflect"
	"strings"

	jsonv2 "encoding/json/v2"

	"google.golang.org/genai"
)

var (
	contentType    = reflect.TypeOf(genai.Content{})
	contentPtrType = reflect.TypeOf((*genai.Content)(nil))
)

// decodeGenerateConfig decodes a generate_content_config map into a
// [*genai.GenerateContentConfig].
//
// Keys are matched to struct fields by a normalized comparison — underscores
// and hyphens are stripped and the result is case-folded — against the field's
// json tag. This accepts snake_case (adk-python files), camelCase (Go json
// tags), and all-lowercase (yaml.v4's default) spellings.
func decodeGenerateConfig(m map[string]any) (*genai.GenerateContentConfig, error) {
	return decodeGenai[genai.GenerateContentConfig](m, "generate_content_config")
}

// decodeGenai decodes m into a new *T using the normalizing rules described on
// decodeGenerateConfig. path seeds nested error messages.
func decodeGenai[T any](m map[string]any, path string) (*T, error) {
	var out T
	if err := decodeStruct(reflect.ValueOf(&out).Elem(), m, path); err != nil {
		return nil, err
	}
	return &out, nil
}

// normalizeKey strips '_' and '-' and lowercases the result.
func normalizeKey(k string) string {
	k = strings.ReplaceAll(k, "_", "")
	k = strings.ReplaceAll(k, "-", "")
	return strings.ToLower(k)
}

type fieldInfo struct {
	index []int
}

func collectFields(t reflect.Type, prefix []int, out map[string]fieldInfo) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" { // unexported
			continue
		}
		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name := strings.Split(tag, ",")[0]
		idx := append(append([]int{}, prefix...), i)

		if name == "" {
			ft := f.Type
			if ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			if f.Anonymous && ft.Kind() == reflect.Struct {
				collectFields(ft, idx, out)
				continue
			}
			name = f.Name
		}
		out[normalizeKey(name)] = fieldInfo{index: idx}
	}
}

func decodeStruct(v reflect.Value, m map[string]any, path string) error {
	fields := make(map[string]fieldInfo)
	collectFields(v.Type(), nil, fields)

	seen := make(map[string]string, len(m))
	for k, val := range m {
		norm := normalizeKey(k)
		fi, ok := fields[norm]
		if !ok {
			return fmt.Errorf("%s: %w %q", path, ErrUnknownField, k)
		}
		if prev, dup := seen[norm]; dup {
			return fmt.Errorf("%s: %w: %q and %q", path, ErrAmbiguousField, prev, k)
		}
		seen[norm] = k

		if err := decodeValue(v.FieldByIndex(fi.index), val, path+"."+k); err != nil {
			return err
		}
	}
	return nil
}

func decodeValue(fv reflect.Value, val any, path string) error {
	if val == nil {
		// A YAML/JSON null leaves the field at its zero value.
		return nil
	}
	if fv.Type() == contentPtrType {
		c, err := decodeContent(val, path)
		if err != nil {
			return err
		}
		fv.Set(reflect.ValueOf(c))
		return nil
	}
	if fv.Type() == contentType {
		c, err := decodeContent(val, path)
		if err != nil {
			return err
		}
		fv.Set(reflect.ValueOf(*c))
		return nil
	}

	switch fv.Kind() {
	case reflect.Pointer:
		elem := reflect.New(fv.Type().Elem())
		if err := decodeValue(elem.Elem(), val, path); err != nil {
			return err
		}
		fv.Set(elem)
		return nil
	case reflect.Struct:
		m, ok := asMap(val)
		if !ok {
			return fmt.Errorf("%s: want map, got %T", path, val)
		}
		return decodeStruct(fv, m, path)
	case reflect.Slice:
		return decodeSlice(fv, val, path)
	case reflect.Map:
		return decodeMapField(fv, val, path)
	case reflect.Interface:
		if fv.NumMethod() == 0 {
			fv.Set(reflect.ValueOf(normalizeValue(val)))
			return nil
		}
		return scalarConvert(fv, val, path)
	default:
		return scalarConvert(fv, val, path)
	}
}

func decodeSlice(fv reflect.Value, val any, path string) error {
	list, ok := val.([]any)
	if !ok {
		return fmt.Errorf("%s: want list, got %T", path, val)
	}
	elemType := fv.Type().Elem()
	base := elemType
	if base.Kind() == reflect.Pointer {
		base = base.Elem()
	}
	if base.Kind() != reflect.Struct {
		return scalarConvert(fv, val, path)
	}

	out := reflect.MakeSlice(fv.Type(), len(list), len(list))
	for i, item := range list {
		ipath := fmt.Sprintf("%s[%d]", path, i)
		if elemType.Kind() == reflect.Pointer {
			p := reflect.New(elemType.Elem())
			if err := decodeValue(p.Elem(), item, ipath); err != nil {
				return err
			}
			out.Index(i).Set(p)
			continue
		}
		if err := decodeValue(out.Index(i), item, ipath); err != nil {
			return err
		}
	}
	fv.Set(out)
	return nil
}

func decodeMapField(fv reflect.Value, val any, path string) error {
	m, ok := asMap(val)
	if !ok {
		return fmt.Errorf("%s: want map, got %T", path, val)
	}
	if fv.Type().Key().Kind() != reflect.String {
		return scalarConvert(fv, val, path)
	}

	valType := fv.Type().Elem()
	base := valType
	if base.Kind() == reflect.Pointer {
		base = base.Elem()
	}
	if base.Kind() != reflect.Struct {
		return scalarConvert(fv, val, path)
	}

	out := reflect.MakeMapWithSize(fv.Type(), len(m))
	for k, v := range m {
		vpath := path + "." + k
		if valType.Kind() == reflect.Pointer {
			p := reflect.New(base)
			if err := decodeValue(p.Elem(), v, vpath); err != nil {
				return err
			}
			out.SetMapIndex(reflect.ValueOf(k), p)
			continue
		}
		ev := reflect.New(base).Elem()
		if err := decodeValue(ev, v, vpath); err != nil {
			return err
		}
		out.SetMapIndex(reflect.ValueOf(k), ev)
	}
	fv.Set(out)
	return nil
}

// decodeContent implements the system-instruction tri-shape: a string becomes a
// system-role Content, a map is decoded as a Content, and a list becomes Parts.
func decodeContent(v any, path string) (*genai.Content, error) {
	switch t := v.(type) {
	case string:
		return genai.NewContentFromText(t, "system"), nil
	case []any:
		parts := make([]*genai.Part, 0, len(t))
		for i, item := range t {
			var p genai.Part
			if err := decodeValue(reflect.ValueOf(&p).Elem(), item, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return nil, err
			}
			parts = append(parts, &p)
		}
		return &genai.Content{Role: "system", Parts: parts}, nil
	case map[string]any:
		var c genai.Content
		if err := decodeStruct(reflect.ValueOf(&c).Elem(), t, path); err != nil {
			return nil, err
		}
		return &c, nil
	default:
		return nil, fmt.Errorf("%s: want string, map, or list, got %T", path, v)
	}
}

// scalarConvert converts val into fv via an encoding/json/v2 round-trip. v2
// rejects duplicate object names, which also helps surface malformed inputs.
func scalarConvert(fv reflect.Value, val any, path string) error {
	b, err := jsonv2.Marshal(val)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if err := jsonv2.Unmarshal(b, fv.Addr().Interface()); err != nil {
		return fmt.Errorf("%s: want %s, got %T: %w", path, fv.Type(), val, err)
	}
	return nil
}
