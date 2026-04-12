package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	outputTable = "table"
	outputJSON  = "json"
)

func normalizeOutputMode(raw string) (string, error) {
	mode := strings.ToLower(strings.TrimSpace(raw))
	if mode == "" {
		return outputTable, nil
	}
	switch mode {
	case outputTable, outputJSON:
		return mode, nil
	default:
		return "", fmt.Errorf("invalid output mode %q (expected table|json)", raw)
	}
}

func printJSON(value any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}

func printTable(headers []string, rows [][]string) {
	if len(headers) == 0 {
		return
	}
	widths := make([]int, len(headers))
	for i, header := range headers {
		widths[i] = len(header)
	}

	truncated := make([][]string, len(rows))
	for i, row := range rows {
		current := make([]string, len(headers))
		for col := range headers {
			value := ""
			if col < len(row) {
				value = truncateCell(strings.TrimSpace(row[col]), 64)
			}
			current[col] = value
			if len(value) > widths[col] {
				widths[col] = len(value)
			}
		}
		truncated[i] = current
	}

	printRow(headers, widths)
	separator := make([]string, len(headers))
	for i := range headers {
		separator[i] = strings.Repeat("-", widths[i])
	}
	printRow(separator, widths)
	for _, row := range truncated {
		printRow(row, widths)
	}
}

func printRow(cols []string, widths []int) {
	for i, col := range cols {
		if i > 0 {
			fmt.Print("  ")
		}
		fmt.Print(padRight(col, widths[i]))
	}
	fmt.Println()
}

func padRight(value string, width int) string {
	if len(value) >= width {
		return value
	}
	return value + strings.Repeat(" ", width-len(value))
}

func truncateCell(value string, max int) string {
	if max <= 0 || len(value) <= max {
		return value
	}
	if max <= 3 {
		return value[:max]
	}
	return value[:max-3] + "..."
}

func printMapAsKeyValues(value map[string]any) {
	if len(value) == 0 {
		fmt.Println("(empty)")
		return
	}
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Printf("%s: %s\n", key, asString(value[key]))
	}
}

func asString(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case time.Time:
		return v.UTC().Format(time.RFC3339Nano)
	case fmt.Stringer:
		return strings.TrimSpace(v.String())
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	case float32:
		f := float64(v)
		if f == float64(int64(f)) {
			return strconv.FormatInt(int64(f), 10)
		}
		return strconv.FormatFloat(f, 'f', -1, 64)
	case bool:
		if v {
			return "true"
		}
		return "false"
	default:
		raw, err := json.Marshal(v)
		if err == nil {
			return string(raw)
		}
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

// asBool is a test-only utility kept for output_test.go coverage.
// It is intentionally not called from production code.
//lint:ignore U1000 this is a test-only utility function
func asBool(value any, fallback bool) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		default:
			return fallback
		}
	default:
		return fallback
	}
}

func asInt(value any, fallback int) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return fallback
		}
		return parsed
	default:
		return fallback
	}
}

func asSlice(value any) []any {
	switch v := value.(type) {
	case []any:
		return v
	case []map[string]any:
		out := make([]any, len(v))
		for i := range v {
			out[i] = v[i]
		}
		return out
	default:
		return nil
	}
}

func asMap(value any) map[string]any {
	m, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	return m
}

func findFirst(m map[string]any, keys ...string) (any, bool) {
	if m == nil {
		return nil, false
	}
	for _, target := range keys {
		for k, v := range m {
			if strings.EqualFold(strings.TrimSpace(k), strings.TrimSpace(target)) {
				return v, true
			}
		}
	}
	return nil, false
}

func findString(m map[string]any, keys ...string) (string, bool) {
	v, ok := findFirst(m, keys...)
	if !ok {
		return "", false
	}
	return asString(v), true
}

func requireArg(raw, name string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}

func requireInt(value int, name string) (int, error) {
	if value <= 0 {
		return 0, errors.New(name + " must be greater than zero")
	}
	return value, nil
}
