package yaml

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

var durationType = reflect.TypeOf(time.Duration(0))

// Unmarshal decodes YAML bytes into the supplied destination pointer.
func Unmarshal(data []byte, v any) error {
	if v == nil {
		return fmt.Errorf("unmarshal target cannot be nil")
	}

	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return fmt.Errorf("unmarshal target must be a non-nil pointer")
	}

	node, err := Parse(data)
	if err != nil {
		return err
	}

	return decodeInto(rv.Elem(), nodeToAny(node))
}

func nodeToAny(node Node) any {
	switch n := node.(type) {
	case *NodeScalar:
		return n.Value
	case *NodeSequence:
		out := make([]any, 0, len(n.Items))
		for _, item := range n.Items {
			out = append(out, nodeToAny(item))
		}
		return out
	case *NodeMap:
		out := make(map[string]any, len(n.Entries))
		for _, key := range n.Order {
			out[key] = nodeToAny(n.Entries[key])
		}
		return out
	default:
		return nil
	}
}

func decodeInto(dst reflect.Value, src any) error {
	if !dst.CanSet() {
		return fmt.Errorf("destination is not settable")
	}

	for dst.Kind() == reflect.Pointer {
		if dst.IsNil() {
			dst.Set(reflect.New(dst.Type().Elem()))
		}
		dst = dst.Elem()
	}

	if dst.Type() == durationType {
		d, err := coerceDuration(src)
		if err != nil {
			return err
		}
		dst.SetInt(int64(d))
		return nil
	}

	switch dst.Kind() {
	case reflect.Struct:
		return decodeStruct(dst, src)
	case reflect.Map:
		return decodeMap(dst, src)
	case reflect.Slice:
		return decodeSlice(dst, src)
	case reflect.Array:
		return decodeArray(dst, src)
	case reflect.Interface:
		dst.Set(reflect.ValueOf(src))
		return nil
	case reflect.String:
		s, err := coerceString(src)
		if err != nil {
			return err
		}
		dst.SetString(s)
		return nil
	case reflect.Bool:
		b, err := coerceBool(src)
		if err != nil {
			return err
		}
		dst.SetBool(b)
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i, err := coerceInt(src, dst.Type().Bits())
		if err != nil {
			return err
		}
		dst.SetInt(i)
		return nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		u, err := coerceUint(src, dst.Type().Bits())
		if err != nil {
			return err
		}
		dst.SetUint(u)
		return nil
	case reflect.Float32, reflect.Float64:
		f, err := coerceFloat(src, dst.Type().Bits())
		if err != nil {
			return err
		}
		dst.SetFloat(f)
		return nil
	default:
		return fmt.Errorf("unsupported destination kind: %s", dst.Kind())
	}
}

func decodeStruct(dst reflect.Value, src any) error {
	srcMap, ok := src.(map[string]any)
	if !ok {
		return fmt.Errorf("expected map for struct, got %T", src)
	}

	for i := 0; i < dst.NumField(); i++ {
		field := dst.Type().Field(i)
		if field.PkgPath != "" {
			continue
		}

		name, skip := yamlFieldName(field)
		if skip {
			continue
		}

		value, found := lookupMapValue(srcMap, name, field.Name)
		if !found {
			continue
		}

		if err := decodeInto(dst.Field(i), value); err != nil {
			return fmt.Errorf("field %s: %w", field.Name, err)
		}
	}
	return nil
}

func decodeMap(dst reflect.Value, src any) error {
	srcMap, ok := src.(map[string]any)
	if !ok {
		return fmt.Errorf("expected map input, got %T", src)
	}
	if dst.IsNil() {
		dst.Set(reflect.MakeMap(dst.Type()))
	}

	for key, value := range srcMap {
		keyRV := reflect.New(dst.Type().Key()).Elem()
		if err := decodeInto(keyRV, key); err != nil {
			return fmt.Errorf("map key %q: %w", key, err)
		}

		valRV := reflect.New(dst.Type().Elem()).Elem()
		if err := decodeInto(valRV, value); err != nil {
			return fmt.Errorf("map key %q: %w", key, err)
		}
		dst.SetMapIndex(keyRV, valRV)
	}
	return nil
}

func decodeSlice(dst reflect.Value, src any) error {
	items, ok := src.([]any)
	if !ok {
		return fmt.Errorf("expected sequence, got %T", src)
	}

	out := reflect.MakeSlice(dst.Type(), len(items), len(items))
	for i := range items {
		if err := decodeInto(out.Index(i), items[i]); err != nil {
			return fmt.Errorf("index %d: %w", i, err)
		}
	}
	dst.Set(out)
	return nil
}

func decodeArray(dst reflect.Value, src any) error {
	items, ok := src.([]any)
	if !ok {
		return fmt.Errorf("expected sequence, got %T", src)
	}

	n := dst.Len()
	if len(items) < n {
		n = len(items)
	}
	for i := 0; i < n; i++ {
		if err := decodeInto(dst.Index(i), items[i]); err != nil {
			return fmt.Errorf("index %d: %w", i, err)
		}
	}
	return nil
}

func yamlFieldName(field reflect.StructField) (name string, skip bool) {
	tag := field.Tag.Get("yaml")
	if tag == "-" {
		return "", true
	}
	if tag != "" {
		name = strings.TrimSpace(strings.Split(tag, ",")[0])
		if name == "-" {
			return "", true
		}
		if name != "" {
			return name, false
		}
	}
	return toSnakeCase(field.Name), false
}

func lookupMapValue(src map[string]any, preferred string, fallback string) (any, bool) {
	if value, ok := src[preferred]; ok {
		return value, true
	}
	if value, ok := src[fallback]; ok {
		return value, true
	}
	lowerPreferred := strings.ToLower(preferred)
	lowerFallback := strings.ToLower(fallback)
	for key, value := range src {
		k := strings.ToLower(key)
		if k == lowerPreferred || k == lowerFallback {
			return value, true
		}
	}
	return nil, false
}

func coerceString(src any) (string, error) {
	switch v := src.(type) {
	case string:
		return v, nil
	case fmt.Stringer:
		return v.String(), nil
	case bool:
		return strconv.FormatBool(v), nil
	case int:
		return strconv.Itoa(v), nil
	case int8, int16, int32, int64:
		return fmt.Sprintf("%d", v), nil
	case uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf("%d", v), nil
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 32), nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	default:
		return "", fmt.Errorf("cannot coerce %T to string", src)
	}
}

func coerceBool(src any) (bool, error) {
	switch v := src.(type) {
	case bool:
		return v, nil
	case string:
		s := strings.TrimSpace(strings.ToLower(v))
		switch s {
		case "true", "yes", "on", "1":
			return true, nil
		case "false", "no", "off", "0":
			return false, nil
		default:
			return false, fmt.Errorf("invalid bool: %q", v)
		}
	default:
		return false, fmt.Errorf("cannot coerce %T to bool", src)
	}
}

func coerceInt(src any, bits int) (int64, error) {
	switch v := src.(type) {
	case int:
		return int64(v), nil
	case int8:
		return int64(v), nil
	case int16:
		return int64(v), nil
	case int32:
		return int64(v), nil
	case int64:
		return v, nil
	case uint:
		if v > uint(^uint64(0)>>1) {
			return 0, fmt.Errorf("uint value out of int range")
		}
		return int64(v), nil
	case uint8:
		return int64(v), nil
	case uint16:
		return int64(v), nil
	case uint32:
		return int64(v), nil
	case uint64:
		if v > uint64(^uint64(0)>>1) {
			return 0, fmt.Errorf("uint value out of int range")
		}
		return int64(v), nil
	case float32:
		return int64(v), nil
	case float64:
		return int64(v), nil
	case string:
		i, err := strconv.ParseInt(strings.TrimSpace(v), 10, bits)
		if err != nil {
			return 0, err
		}
		return i, nil
	default:
		return 0, fmt.Errorf("cannot coerce %T to int", src)
	}
}

func coerceUint(src any, bits int) (uint64, error) {
	switch v := src.(type) {
	case int:
		if v < 0 {
			return 0, fmt.Errorf("negative int cannot be coerced to uint")
		}
		return uint64(v), nil
	case int8:
		if v < 0 {
			return 0, fmt.Errorf("negative int8 cannot be coerced to uint")
		}
		return uint64(v), nil
	case int16:
		if v < 0 {
			return 0, fmt.Errorf("negative int16 cannot be coerced to uint")
		}
		return uint64(v), nil
	case int32:
		if v < 0 {
			return 0, fmt.Errorf("negative int32 cannot be coerced to uint")
		}
		return uint64(v), nil
	case int64:
		if v < 0 {
			return 0, fmt.Errorf("negative int64 cannot be coerced to uint")
		}
		return uint64(v), nil
	case uint:
		return uint64(v), nil
	case uint8:
		return uint64(v), nil
	case uint16:
		return uint64(v), nil
	case uint32:
		return uint64(v), nil
	case uint64:
		return v, nil
	case string:
		u, err := strconv.ParseUint(strings.TrimSpace(v), 10, bits)
		if err != nil {
			return 0, err
		}
		return u, nil
	default:
		return 0, fmt.Errorf("cannot coerce %T to uint", src)
	}
}

func coerceFloat(src any, bits int) (float64, error) {
	switch v := src.(type) {
	case float32:
		return float64(v), nil
	case float64:
		return v, nil
	case int:
		return float64(v), nil
	case int8:
		return float64(v), nil
	case int16:
		return float64(v), nil
	case int32:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case uint:
		return float64(v), nil
	case uint8:
		return float64(v), nil
	case uint16:
		return float64(v), nil
	case uint32:
		return float64(v), nil
	case uint64:
		return float64(v), nil
	case string:
		return strconv.ParseFloat(strings.TrimSpace(v), bits)
	default:
		return 0, fmt.Errorf("cannot coerce %T to float", src)
	}
}

func coerceDuration(src any) (time.Duration, error) {
	switch v := src.(type) {
	case time.Duration:
		return v, nil
	case string:
		return time.ParseDuration(strings.TrimSpace(v))
	case int:
		return time.Duration(v), nil
	case int64:
		return time.Duration(v), nil
	default:
		return 0, fmt.Errorf("cannot coerce %T to time.Duration", src)
	}
}

func toSnakeCase(s string) string {
	if s == "" {
		return s
	}

	var b strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			b.WriteByte('_')
		}
		b.WriteRune(r)
	}
	return strings.ToLower(b.String())
}
