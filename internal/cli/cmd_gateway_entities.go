package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

func runService(args []string) error {
	return runEntityCommand("service", "/admin/api/v1/services", args)
}

func runRoute(args []string) error {
	return runEntityCommand("route", "/admin/api/v1/routes", args)
}

func runUpstream(args []string) error {
	return runEntityCommand("upstream", "/admin/api/v1/upstreams", args)
}

func runEntityCommand(name, basePath string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing %s subcommand (expected: list|add|get|update|delete)", name)
	}
	switch args[0] {
	case "list":
		return runEntityList(name, basePath, args[1:])
	case "add":
		return runEntityAdd(name, basePath, args[1:])
	case "get":
		return runEntityGet(name, basePath, args[1:])
	case "update":
		return runEntityUpdate(name, basePath, args[1:])
	case "delete":
		return runEntityDelete(name, basePath, args[1:])
	default:
		return fmt.Errorf("unknown %s subcommand %q", name, args[0])
	}
}

func runEntityList(name, basePath string, args []string) error {
	fs := flag.NewFlagSet(name+" list", flag.ContinueOnError)
	common := addAdminCommandFlags(fs)
	if err := fs.Parse(args); err != nil {
		return err
	}

	client, mode, err := resolveAdminCommand(common)
	if err != nil {
		return err
	}
	result, err := client.call(http.MethodGet, basePath, nil, nil)
	if err != nil {
		return err
	}
	if mode == outputJSON {
		return printJSON(result)
	}

	items := asSlice(result)
	if len(items) == 0 {
		fmt.Printf("No %ss found.\n", name)
		return nil
	}
	switch name {
	case "service":
		rows := make([][]string, 0, len(items))
		for _, item := range items {
			m := asMap(item)
			rows = append(rows, []string{
				firstString(m, "id", "ID"),
				firstString(m, "name", "Name"),
				firstString(m, "protocol", "Protocol"),
				firstString(m, "upstream", "Upstream"),
			})
		}
		printTable([]string{"ID", "NAME", "PROTOCOL", "UPSTREAM"}, rows)
	case "route":
		rows := make([][]string, 0, len(items))
		for _, item := range items {
			m := asMap(item)
			rows = append(rows, []string{
				firstString(m, "id", "ID"),
				firstString(m, "name", "Name"),
				firstString(m, "service", "Service"),
				firstString(m, "paths", "Paths"),
				firstString(m, "methods", "Methods"),
				firstString(m, "priority", "Priority"),
			})
		}
		printTable([]string{"ID", "NAME", "SERVICE", "PATHS", "METHODS", "PRIORITY"}, rows)
	case "upstream":
		rows := make([][]string, 0, len(items))
		for _, item := range items {
			m := asMap(item)
			targetCount := 0
			if raw, ok := findFirst(m, "targets", "Targets"); ok {
				targetCount = len(asSlice(raw))
			}
			rows = append(rows, []string{
				firstString(m, "id", "ID"),
				firstString(m, "name", "Name"),
				firstString(m, "algorithm", "Algorithm"),
				asString(targetCount),
			})
		}
		printTable([]string{"ID", "NAME", "ALGORITHM", "TARGETS"}, rows)
	default:
		return printJSON(result)
	}
	return nil
}

func runEntityAdd(name, basePath string, args []string) error {
	fs := flag.NewFlagSet(name+" add", flag.ContinueOnError)
	common := addAdminCommandFlags(fs)
	filePath := fs.String("file", "", "path to JSON payload file")
	body := fs.String("body", "", "inline JSON payload")
	if err := fs.Parse(args); err != nil {
		return err
	}

	payload, err := loadJSONPayload(*filePath, *body)
	if err != nil {
		return err
	}
	client, mode, err := resolveAdminCommand(common)
	if err != nil {
		return err
	}
	result, err := client.call(http.MethodPost, basePath, nil, payload)
	if err != nil {
		return err
	}
	if mode == outputJSON {
		return printJSON(result)
	}
	printMapAsKeyValues(asMap(result))
	return nil
}

func runEntityGet(name, basePath string, args []string) error {
	fs := flag.NewFlagSet(name+" get", flag.ContinueOnError)
	common := addAdminCommandFlags(fs)
	id := fs.String("id", "", name+" id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	entityID := strings.TrimSpace(*id)
	if entityID == "" && fs.NArg() > 0 {
		entityID = strings.TrimSpace(fs.Arg(0))
	}
	if entityID == "" {
		return errors.New(name + " id is required (use --id or positional <id>)")
	}

	client, mode, err := resolveAdminCommand(common)
	if err != nil {
		return err
	}
	result, err := client.call(http.MethodGet, basePath+"/"+url.PathEscape(entityID), nil, nil)
	if err != nil {
		return err
	}
	if mode == outputJSON {
		return printJSON(result)
	}
	printMapAsKeyValues(asMap(result))
	return nil
}

func runEntityUpdate(name, basePath string, args []string) error {
	fs := flag.NewFlagSet(name+" update", flag.ContinueOnError)
	common := addAdminCommandFlags(fs)
	id := fs.String("id", "", name+" id")
	filePath := fs.String("file", "", "path to JSON payload file")
	body := fs.String("body", "", "inline JSON payload")
	if err := fs.Parse(args); err != nil {
		return err
	}
	entityID := strings.TrimSpace(*id)
	if entityID == "" && fs.NArg() > 0 {
		entityID = strings.TrimSpace(fs.Arg(0))
	}
	if entityID == "" {
		return errors.New(name + " id is required (use --id or positional <id>)")
	}
	payload, err := loadJSONPayload(*filePath, *body)
	if err != nil {
		return err
	}

	client, mode, err := resolveAdminCommand(common)
	if err != nil {
		return err
	}
	result, err := client.call(http.MethodPut, basePath+"/"+url.PathEscape(entityID), nil, payload)
	if err != nil {
		return err
	}
	if mode == outputJSON {
		return printJSON(result)
	}
	printMapAsKeyValues(asMap(result))
	return nil
}

func runEntityDelete(name, basePath string, args []string) error {
	fs := flag.NewFlagSet(name+" delete", flag.ContinueOnError)
	common := addAdminCommandFlags(fs)
	id := fs.String("id", "", name+" id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	entityID := strings.TrimSpace(*id)
	if entityID == "" && fs.NArg() > 0 {
		entityID = strings.TrimSpace(fs.Arg(0))
	}
	if entityID == "" {
		return errors.New(name + " id is required (use --id or positional <id>)")
	}

	client, mode, err := resolveAdminCommand(common)
	if err != nil {
		return err
	}
	_, err = client.call(http.MethodDelete, basePath+"/"+url.PathEscape(entityID), nil, nil)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"deleted": true,
		"id":      entityID,
		"type":    name,
	}
	if mode == outputJSON {
		return printJSON(payload)
	}
	fmt.Printf("%s deleted: %s\n", cases.Title(language.Und).String(name), entityID)
	return nil
}

func loadJSONPayload(path, body string) (map[string]any, error) {
	path = strings.TrimSpace(path)
	body = strings.TrimSpace(body)
	if path == "" && body == "" {
		return nil, errors.New("payload is required (use --file or --body)")
	}
	var raw []byte
	var err error
	if path != "" {
		// #nosec G304 -- path is supplied by the CLI administrator for payload upload.
		raw, err = os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read payload file: %w", err)
		}
	} else {
		raw = []byte(body)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("decode JSON payload: %w", err)
	}
	return payload, nil
}
