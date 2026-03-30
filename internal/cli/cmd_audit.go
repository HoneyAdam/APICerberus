package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/APICerberus/APICerebrus/internal/config"
	yamlpkg "github.com/APICerberus/APICerebrus/internal/pkg/yaml"
)

func runAudit(args []string) error {
	if len(args) == 0 {
		return errors.New("missing audit subcommand (expected: search|tail|detail|export|stats|cleanup|retention)")
	}
	switch args[0] {
	case "search":
		return runAuditSearch(args[1:])
	case "tail":
		return runAuditTail(args[1:])
	case "detail":
		return runAuditDetail(args[1:])
	case "export":
		return runAuditExport(args[1:])
	case "stats":
		return runAuditStats(args[1:])
	case "cleanup":
		return runAuditCleanup(args[1:])
	case "retention":
		return runAuditRetention(args[1:])
	default:
		return fmt.Errorf("unknown audit subcommand %q", args[0])
	}
}

func runAuditSearch(args []string) error {
	query, common, err := parseAuditQueryFlags("audit search", args)
	if err != nil {
		return err
	}
	client, mode, err := resolveAdminCommand(common)
	if err != nil {
		return err
	}
	result, err := client.call(http.MethodGet, "/admin/api/v1/audit-logs", query, nil)
	if err != nil {
		return err
	}
	if mode == outputJSON {
		return printJSON(result)
	}
	return printAuditList(result)
}

func runAuditTail(args []string) error {
	fs := flag.NewFlagSet("audit tail", flag.ContinueOnError)
	common := addAdminCommandFlags(fs)
	interval := fs.Duration("interval", 2*time.Second, "poll interval")
	limit := fs.Int("limit", 20, "entries per poll")
	userID := fs.String("user-id", "", "filter by user id")
	route := fs.String("route", "", "filter by route id/name")
	search := fs.String("search", "", "full-text query")
	if err := fs.Parse(args); err != nil {
		return err
	}

	client, mode, err := resolveAdminCommand(common)
	if err != nil {
		return err
	}
	if *interval <= 0 {
		*interval = 2 * time.Second
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	ticker := time.NewTicker(*interval)
	defer ticker.Stop()

	seen := map[string]struct{}{}
	for {
		query := url.Values{}
		query.Set("limit", asString(*limit))
		if strings.TrimSpace(*userID) != "" {
			query.Set("user_id", strings.TrimSpace(*userID))
		}
		if strings.TrimSpace(*route) != "" {
			query.Set("route", strings.TrimSpace(*route))
		}
		if strings.TrimSpace(*search) != "" {
			query.Set("q", strings.TrimSpace(*search))
		}

		result, err := client.call(http.MethodGet, "/admin/api/v1/audit-logs", query, nil)
		if err != nil {
			return err
		}
		payload := asMap(result)
		itemsRaw, _ := findFirst(payload, "entries", "Entries")
		items := asSlice(itemsRaw)

		if mode == outputJSON {
			if len(items) > 0 {
				if err := printJSON(items); err != nil {
					return err
				}
			}
		} else {
			newItems := collectUnseenAuditRows(items, seen)
			if len(newItems) > 0 {
				for _, row := range newItems {
					fmt.Println(row)
				}
			}
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func runAuditDetail(args []string) error {
	fs := flag.NewFlagSet("audit detail", flag.ContinueOnError)
	common := addAdminCommandFlags(fs)
	id := fs.String("id", "", "audit log id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	entryID := strings.TrimSpace(*id)
	if entryID == "" && fs.NArg() > 0 {
		entryID = strings.TrimSpace(fs.Arg(0))
	}
	if entryID == "" {
		return errors.New("audit id is required (use --id or positional <id>)")
	}

	client, mode, err := resolveAdminCommand(common)
	if err != nil {
		return err
	}
	result, err := client.call(http.MethodGet, "/admin/api/v1/audit-logs/"+url.PathEscape(entryID), nil, nil)
	if err != nil {
		return err
	}
	if mode == outputJSON {
		return printJSON(result)
	}
	printMapAsKeyValues(asMap(result))
	return nil
}

func runAuditExport(args []string) error {
	query, common, err := parseAuditQueryFlags("audit export", args)
	if err != nil {
		return err
	}
	format := query.Get("format")
	if strings.TrimSpace(format) == "" {
		query.Set("format", "jsonl")
	}

	client, mode, err := resolveAdminCommand(common)
	if err != nil {
		return err
	}
	result, err := client.call(http.MethodGet, "/admin/api/v1/audit-logs/export", query, nil)
	if err != nil {
		return err
	}
	if mode == outputJSON {
		return printJSON(result)
	}
	fmt.Print(asString(result))
	if !strings.HasSuffix(asString(result), "\n") {
		fmt.Println()
	}
	return nil
}

func runAuditStats(args []string) error {
	query, common, err := parseAuditQueryFlags("audit stats", args)
	if err != nil {
		return err
	}
	client, mode, err := resolveAdminCommand(common)
	if err != nil {
		return err
	}
	result, err := client.call(http.MethodGet, "/admin/api/v1/audit-logs/stats", query, nil)
	if err != nil {
		return err
	}
	if mode == outputJSON {
		return printJSON(result)
	}

	payload := asMap(result)
	fmt.Printf("Total Requests: %s\n", firstString(payload, "total_requests", "TotalRequests"))
	fmt.Printf("Error Requests: %s\n", firstString(payload, "error_requests", "ErrorRequests"))
	fmt.Printf("Error Rate    : %s\n", firstString(payload, "error_rate", "ErrorRate"))
	fmt.Printf("Avg LatencyMs : %s\n", firstString(payload, "avg_latency_ms", "AvgLatencyMS"))

	if routesRaw, ok := findFirst(payload, "top_routes", "TopRoutes"); ok {
		routes := asSlice(routesRaw)
		if len(routes) > 0 {
			fmt.Println("\nTop Routes:")
			rows := make([][]string, 0, len(routes))
			for _, item := range routes {
				route := asMap(item)
				rows = append(rows, []string{
					firstString(route, "route_id", "RouteID"),
					firstString(route, "route_name", "RouteName"),
					firstString(route, "count", "Count"),
				})
			}
			printTable([]string{"ROUTE ID", "ROUTE", "COUNT"}, rows)
		}
	}

	if usersRaw, ok := findFirst(payload, "top_users", "TopUsers"); ok {
		users := asSlice(usersRaw)
		if len(users) > 0 {
			fmt.Println("\nTop Users:")
			rows := make([][]string, 0, len(users))
			for _, item := range users {
				user := asMap(item)
				rows = append(rows, []string{
					firstString(user, "user_id", "UserID"),
					firstString(user, "consumer_name", "ConsumerName"),
					firstString(user, "count", "Count"),
				})
			}
			printTable([]string{"USER ID", "CONSUMER", "COUNT"}, rows)
		}
	}
	return nil
}

func runAuditCleanup(args []string) error {
	fs := flag.NewFlagSet("audit cleanup", flag.ContinueOnError)
	common := addAdminCommandFlags(fs)
	cutoff := fs.String("cutoff", "", "RFC3339 cutoff time")
	olderThanDays := fs.Int("older-than-days", 30, "delete logs older than days when cutoff is not provided")
	batchSize := fs.Int("batch-size", 1000, "delete batch size")
	if err := fs.Parse(args); err != nil {
		return err
	}

	query := url.Values{}
	if strings.TrimSpace(*cutoff) != "" {
		query.Set("cutoff", strings.TrimSpace(*cutoff))
	} else if *olderThanDays > 0 {
		query.Set("older_than_days", asString(*olderThanDays))
	}
	if *batchSize > 0 {
		query.Set("batch_size", asString(*batchSize))
	}

	client, mode, err := resolveAdminCommand(common)
	if err != nil {
		return err
	}
	result, err := client.call(http.MethodDelete, "/admin/api/v1/audit-logs/cleanup", query, nil)
	if err != nil {
		return err
	}
	if mode == outputJSON {
		return printJSON(result)
	}
	printMapAsKeyValues(asMap(result))
	return nil
}

func parseAuditQueryFlags(name string, args []string) (url.Values, *adminCommandFlags, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	common := addAdminCommandFlags(fs)
	userID := fs.String("user-id", "", "filter by user id")
	route := fs.String("route", "", "filter by route id/name")
	method := fs.String("method", "", "filter by method")
	statusMin := fs.Int("status-min", 0, "minimum status code")
	statusMax := fs.Int("status-max", 0, "maximum status code")
	clientIP := fs.String("client-ip", "", "client IP filter")
	blocked := fs.String("blocked", "", "blocked filter (true|false)")
	blockReason := fs.String("block-reason", "", "block reason filter")
	from := fs.String("from", "", "RFC3339 start time")
	to := fs.String("to", "", "RFC3339 end time")
	minLatency := fs.Int("min-latency-ms", 0, "minimum latency in ms")
	search := fs.String("search", "", "full-text search")
	limit := fs.Int("limit", 50, "result limit")
	offset := fs.Int("offset", 0, "result offset")
	format := fs.String("format", "", "export format (csv|json|jsonl)")
	if err := fs.Parse(args); err != nil {
		return nil, nil, err
	}

	query := url.Values{}
	if strings.TrimSpace(*userID) != "" {
		query.Set("user_id", strings.TrimSpace(*userID))
	}
	if strings.TrimSpace(*route) != "" {
		query.Set("route", strings.TrimSpace(*route))
	}
	if strings.TrimSpace(*method) != "" {
		query.Set("method", strings.TrimSpace(*method))
	}
	if *statusMin > 0 {
		query.Set("status_min", asString(*statusMin))
	}
	if *statusMax > 0 {
		query.Set("status_max", asString(*statusMax))
	}
	if strings.TrimSpace(*clientIP) != "" {
		query.Set("client_ip", strings.TrimSpace(*clientIP))
	}
	if strings.TrimSpace(*blocked) != "" {
		query.Set("blocked", strings.TrimSpace(*blocked))
	}
	if strings.TrimSpace(*blockReason) != "" {
		query.Set("block_reason", strings.TrimSpace(*blockReason))
	}
	if strings.TrimSpace(*from) != "" {
		query.Set("date_from", strings.TrimSpace(*from))
	}
	if strings.TrimSpace(*to) != "" {
		query.Set("date_to", strings.TrimSpace(*to))
	}
	if *minLatency > 0 {
		query.Set("min_latency_ms", asString(*minLatency))
	}
	if strings.TrimSpace(*search) != "" {
		query.Set("q", strings.TrimSpace(*search))
	}
	if *limit > 0 {
		query.Set("limit", asString(*limit))
	}
	if *offset > 0 {
		query.Set("offset", asString(*offset))
	}
	if strings.TrimSpace(*format) != "" {
		query.Set("format", strings.TrimSpace(*format))
	}
	return query, common, nil
}

func printAuditList(result any) error {
	payload := asMap(result)
	itemsRaw, ok := findFirst(payload, "entries", "Entries")
	if !ok {
		return printJSON(result)
	}
	items := asSlice(itemsRaw)
	if len(items) == 0 {
		fmt.Println("No audit logs found.")
		return nil
	}
	rows := make([][]string, 0, len(items))
	for _, item := range items {
		entry := asMap(item)
		rows = append(rows, []string{
			firstString(entry, "id", "ID"),
			firstString(entry, "created_at", "CreatedAt"),
			firstString(entry, "method", "Method"),
			firstString(entry, "path", "Path"),
			firstString(entry, "status_code", "StatusCode"),
			firstString(entry, "latency_ms", "LatencyMS"),
			firstString(entry, "user_id", "UserID"),
			firstString(entry, "route_name", "RouteName"),
		})
	}
	printTable([]string{"ID", "TIME", "METHOD", "PATH", "STATUS", "LAT(ms)", "USER", "ROUTE"}, rows)
	if totalRaw, ok := findFirst(payload, "total", "Total"); ok {
		fmt.Printf("\nTotal: %d\n", asInt(totalRaw, len(items)))
	}
	return nil
}

func collectUnseenAuditRows(items []any, seen map[string]struct{}) []string {
	type row struct {
		timestamp string
		line      string
	}
	newRows := make([]row, 0, len(items))
	for _, item := range items {
		entry := asMap(item)
		id := firstString(entry, "id", "ID")
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}

		ts := firstString(entry, "created_at", "CreatedAt")
		method := firstString(entry, "method", "Method")
		path := firstString(entry, "path", "Path")
		status := firstString(entry, "status_code", "StatusCode")
		latency := firstString(entry, "latency_ms", "LatencyMS")
		userID := firstString(entry, "user_id", "UserID")
		route := firstString(entry, "route_name", "RouteName")
		newRows = append(newRows, row{
			timestamp: ts,
			line:      fmt.Sprintf("%s  %s %s  status=%s latency=%sms user=%s route=%s id=%s", ts, method, path, status, latency, userID, route, id),
		})
	}
	sort.Slice(newRows, func(i, j int) bool {
		return newRows[i].timestamp < newRows[j].timestamp
	})
	out := make([]string, 0, len(newRows))
	for _, item := range newRows {
		out = append(out, item.line)
	}
	return out
}

func runAuditRetention(args []string) error {
	if len(args) == 0 {
		return runAuditRetentionShow(nil)
	}
	switch args[0] {
	case "show":
		return runAuditRetentionShow(args[1:])
	case "set":
		return runAuditRetentionSet(args[1:])
	default:
		return fmt.Errorf("unknown audit retention subcommand %q", args[0])
	}
}

func runAuditRetentionShow(args []string) error {
	fs := flag.NewFlagSet("audit retention show", flag.ContinueOnError)
	configPath := fs.String("config", "apicerberus.yaml", "config file path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	fmt.Printf("Config: %s\n", filepath.Clean(*configPath))
	fmt.Printf("Global retention days: %d\n", cfg.Audit.RetentionDays)
	if len(cfg.Audit.RouteRetentionDays) == 0 {
		fmt.Println("Route retention overrides: none")
		return nil
	}
	fmt.Println("Route retention overrides:")
	keys := make([]string, 0, len(cfg.Audit.RouteRetentionDays))
	for k := range cfg.Audit.RouteRetentionDays {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	rows := make([][]string, 0, len(keys))
	for _, key := range keys {
		rows = append(rows, []string{key, asString(cfg.Audit.RouteRetentionDays[key])})
	}
	printTable([]string{"ROUTE", "DAYS"}, rows)
	return nil
}

func runAuditRetentionSet(args []string) error {
	fs := flag.NewFlagSet("audit retention set", flag.ContinueOnError)
	configPath := fs.String("config", "apicerberus.yaml", "config file path")
	days := fs.Int("days", -1, "global retention days")
	route := fs.String("route", "", "route id/name override")
	routeDays := fs.Int("route-days", -1, "route retention days")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	changed := false
	if *days >= 0 {
		cfg.Audit.RetentionDays = *days
		changed = true
	}
	routeName := strings.TrimSpace(*route)
	if routeName != "" {
		if *routeDays < 0 {
			return errors.New("--route-days must be provided when --route is set")
		}
		if cfg.Audit.RouteRetentionDays == nil {
			cfg.Audit.RouteRetentionDays = map[string]int{}
		}
		cfg.Audit.RouteRetentionDays[routeName] = *routeDays
		changed = true
	}
	if !changed {
		return errors.New("no changes provided (use --days and/or --route --route-days)")
	}

	raw, err := yamlpkg.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal updated config: %w", err)
	}
	if err := os.WriteFile(*configPath, raw, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	fmt.Printf("Updated audit retention in %s\n", filepath.Clean(*configPath))
	return nil
}
