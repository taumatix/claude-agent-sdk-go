package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"
)

// NewTypedMCPTool builds a tool whose arguments are the Go type T: the JSON Schema the model
// sees is derived from T, and the handler receives the arguments already decoded.
//
// Field names follow the `json` tag. A field is required unless it is a pointer or its tag
// says `omitempty` or `omitzero`. A `description:"..."` tag becomes the property's description
// and `enum:"a,b"` restricts a string or number field to the listed values. Supported kinds are
// bool, integers, floats, string, slices and arrays, maps with string keys, structs and
// pointers to them; anything else (channels, funcs, interfaces other than any) is a schema
// error reported when the tool is built, as a panic, since it is a programming mistake.
func NewTypedMCPTool[T any](name, description string, handler func(ctx context.Context, in T) (MCPToolResult, error)) MCPTool {
	var zero T
	schema, err := schemaFor(reflect.TypeOf(&zero).Elem(), "", nil)
	if err != nil {
		panic(fmt.Sprintf("agent: tool %q: %v", name, err))
	}
	if schema["type"] != "object" {
		panic(fmt.Sprintf("agent: tool %q: arguments must be a struct or map, got %s", name, schema["type"]))
	}
	raw, _ := json.Marshal(schema)
	return MCPTool{
		Name:        name,
		Description: description,
		InputSchema: raw,
		Handler: func(ctx context.Context, args json.RawMessage) (MCPToolResult, error) {
			var in T
			if err := json.Unmarshal(args, &in); err != nil {
				return MCPToolResult{}, fmt.Errorf("invalid arguments: %w", err)
			}
			return handler(ctx, in)
		},
	}
}

func schemaFor(t reflect.Type, enum string, path map[reflect.Type]bool) (map[string]any, error) {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == reflect.TypeOf(time.Time{}) {
		return map[string]any{"type": "string", "format": "date-time"}, nil
	}
	out := map[string]any{}
	switch t.Kind() {
	case reflect.Bool:
		out["type"] = "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		out["type"] = "integer"
	case reflect.Float32, reflect.Float64:
		out["type"] = "number"
	case reflect.String:
		out["type"] = "string"
	case reflect.Slice, reflect.Array:
		if t == reflect.TypeOf(json.RawMessage(nil)) {
			return map[string]any{}, nil
		}
		items, err := schemaFor(t.Elem(), "", path)
		if err != nil {
			return nil, err
		}
		out["type"], out["items"] = "array", items
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			return nil, fmt.Errorf("map key must be a string, got %s", t.Key())
		}
		values, err := schemaFor(t.Elem(), "", path)
		if err != nil {
			return nil, err
		}
		out["type"], out["additionalProperties"] = "object", values
	case reflect.Interface:
		if t.NumMethod() != 0 {
			return nil, fmt.Errorf("unsupported interface type %s", t)
		}
		return map[string]any{}, nil
	case reflect.Struct:
		if path[t] {
			return nil, fmt.Errorf("recursive type %s cannot be described without $ref", t)
		}
		next := map[reflect.Type]bool{t: true}
		for k := range path {
			next[k] = true
		}
		path = next
		props := map[string]any{}
		required := []string{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			tag := f.Tag.Get("json")
			if tag == "-" {
				continue
			}
			name, opts, _ := strings.Cut(tag, ",")
			if name == "" {
				name = f.Name
			}
			prop, err := schemaFor(f.Type, f.Tag.Get("enum"), path)
			if err != nil {
				return nil, fmt.Errorf("field %s: %w", f.Name, err)
			}
			if d := f.Tag.Get("description"); d != "" {
				prop["description"] = d
			}
			props[name] = prop
			optional := f.Type.Kind() == reflect.Pointer ||
				strings.Contains(","+opts+",", ",omitempty,") || strings.Contains(","+opts+",", ",omitzero,")
			if !optional {
				required = append(required, name)
			}
		}
		out["type"], out["properties"] = "object", props
		if len(required) > 0 {
			out["required"] = required
		}
	default:
		return nil, fmt.Errorf("unsupported type %s", t)
	}
	if enum != "" {
		vals := []any{}
		for _, v := range strings.Split(enum, ",") {
			switch out["type"] {
			case "integer", "number":
				var n json.Number = json.Number(strings.TrimSpace(v))
				vals = append(vals, n)
			default:
				vals = append(vals, v)
			}
		}
		out["enum"] = vals
	}
	return out, nil
}
