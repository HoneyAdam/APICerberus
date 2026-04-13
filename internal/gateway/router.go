package gateway

import (
	"context"
	"errors"
	"fmt"
	stdpath "path"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/APICerberus/APICerebrus/internal/config"
	"net/http"
)

var ErrNoRouteMatched = errors.New("no route matched")

type paramsKey struct{}

// Params returns path parameters extracted during routing.
func Params(req *http.Request) map[string]string {
	if req == nil {
		return map[string]string{}
	}
	raw := req.Context().Value(paramsKey{})
	if raw == nil {
		return map[string]string{}
	}
	params, ok := raw.(map[string]string)
	if !ok || params == nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(params))
	for k, v := range params {
		out[k] = v
	}
	return out
}

// Router keeps host -> method -> path lookup trees.
type Router struct {
	hosts       map[string]*MethodTree
	defaultTree *MethodTree
	regexRoutes []*compiledRoute
	mu          sync.RWMutex
}

// MethodTree keeps method-scoped radix roots plus a wildcard any-method root.
type MethodTree struct {
	trees map[string]*radixNode
	any   *radixNode
}

// radixNode is a radix-style segment trie node.
type radixNode struct {
	path     string
	children []*radixNode
	route    *config.Route
	service  *config.Service
	isWild   bool
	paramKey string
}

type compiledRoute struct {
	host       string
	methods    map[string]struct{}
	re         *regexp.Regexp
	route      *config.Route
	service    *config.Service
	priority   int
	patternLen int
}

func (c *compiledRoute) matches(host, method, path string) bool {
	if c.host != "" && c.host != host {
		return false
	}
	if _, ok := c.methods["*"]; !ok {
		if _, ok := c.methods[method]; !ok {
			return false
		}
	}
	return c.re.MatchString(path)
}

type pathKind int

const (
	pathKindExact pathKind = iota
	pathKindPrefix
	pathKindRegex
)

type routeBinding struct {
	host    string
	method  string
	path    string
	kind    pathKind
	route   *config.Route
	service *config.Service
}

// NewRouter builds a new router from route/service snapshots.
func NewRouter(routes []config.Route, services []config.Service) (*Router, error) {
	r := &Router{}
	if err := r.Rebuild(routes, services); err != nil {
		return nil, err
	}
	return r, nil
}

const (
	maxPathLength   = 8192 // Reject paths longer than 8KB (prevents stack overflow)
	maxPathSegments = 256  // Reject paths with excessive segments
	maxRegexLength  = 1024 // Prevents ReDoS via excessively long patterns (CWE-1333)
)

// Match finds route/service for request and applies strip_path when configured.
func (r *Router) Match(req *http.Request) (*config.Route, *config.Service, error) {
	if req == nil {
		return nil, nil, ErrNoRouteMatched
	}

	host := normalizeHost(req.Host)
	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		method = http.MethodGet
	}
	path := normalizePath(req.URL.Path)

	// Reject adversarial paths: too long, too many segments, or embedded null bytes (CWE-20)
	if len(path) > maxPathLength {
		return nil, nil, ErrNoRouteMatched
	}
	if strings.ContainsRune(path, '\x00') {
		return nil, nil, ErrNoRouteMatched
	}
	if n := strings.Count(path, "/"); n > maxPathSegments {
		return nil, nil, ErrNoRouteMatched
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	if tree, ok := r.hosts[host]; ok {
		if route, service, params, ok := tree.match(method, path); ok {
			setRequestParams(req, params)
			applyStripPath(req, route, path)
			return route, service, nil
		}
	}

	if route, service, params, ok := r.defaultTree.match(method, path); ok {
		setRequestParams(req, params)
		applyStripPath(req, route, path)
		return route, service, nil
	}

	for _, cr := range r.regexRoutes {
		if cr.matches(host, method, path) {
			setRequestParams(req, map[string]string{})
			applyStripPath(req, cr.route, path)
			return cr.route, cr.service, nil
		}
	}

	return nil, nil, ErrNoRouteMatched
}

// Rebuild atomically swaps the whole router state for hot reload.
func (r *Router) Rebuild(routes []config.Route, services []config.Service) error {
	routeCopy := append([]config.Route(nil), routes...)
	serviceCopy := append([]config.Service(nil), services...)

	serviceByName := make(map[string]*config.Service, len(serviceCopy))
	for i := range serviceCopy {
		s := &serviceCopy[i]
		if s.Name != "" {
			serviceByName[s.Name] = s
		}
		if s.ID != "" {
			serviceByName[s.ID] = s
		}
	}

	regular := make([]routeBinding, 0)
	regexRoutes := make([]*compiledRoute, 0)

	for i := range routeCopy {
		route := &routeCopy[i]
		service, ok := serviceByName[route.Service]
		if !ok {
			return fmt.Errorf("route %q references unknown service %q", route.Name, route.Service)
		}

		hosts := normalizeHosts(route.Hosts)
		methods := normalizeMethods(route.Methods)
		for _, host := range hosts {
			for _, method := range methods {
				for _, rawPath := range route.Paths {
					path := normalizePath(rawPath)
					kind := classifyPath(path)

					if kind == pathKindRegex {
						re, err := compileRegex(path)
						if err != nil {
							return fmt.Errorf("route %q regex path %q: %w", route.Name, rawPath, err)
						}
						regexRoutes = append(regexRoutes, &compiledRoute{
							host:       host,
							methods:    methodsToMap([]string{method}),
							re:         re,
							route:      route,
							service:    service,
							priority:   route.Priority,
							patternLen: len(path),
						})
						continue
					}

					regular = append(regular, routeBinding{
						host:    host,
						method:  method,
						path:    path,
						kind:    kind,
						route:   route,
						service: service,
					})
				}
			}
		}
	}

	sort.SliceStable(regular, func(i, j int) bool {
		if regular[i].kind != regular[j].kind {
			return regular[i].kind < regular[j].kind
		}
		if regular[i].route.Priority != regular[j].route.Priority {
			return regular[i].route.Priority > regular[j].route.Priority
		}
		if len(regular[i].path) != len(regular[j].path) {
			return len(regular[i].path) > len(regular[j].path)
		}
		return regular[i].route.Name < regular[j].route.Name
	})

	sort.SliceStable(regexRoutes, func(i, j int) bool {
		iHostSpecific := regexRoutes[i].host != ""
		jHostSpecific := regexRoutes[j].host != ""
		if iHostSpecific != jHostSpecific {
			return iHostSpecific
		}
		if regexRoutes[i].priority != regexRoutes[j].priority {
			return regexRoutes[i].priority > regexRoutes[j].priority
		}
		if regexRoutes[i].patternLen != regexRoutes[j].patternLen {
			return regexRoutes[i].patternLen > regexRoutes[j].patternLen
		}
		return regexRoutes[i].route.Name < regexRoutes[j].route.Name
	})

	hosts := make(map[string]*MethodTree)
	defaultTree := newMethodTree()

	for _, b := range regular {
		tree := defaultTree
		if b.host != "" {
			if existing, ok := hosts[b.host]; ok {
				tree = existing
			} else {
				tree = newMethodTree()
				hosts[b.host] = tree
			}
		}
		tree.insert(b.method, b.path, b.route, b.service)
	}

	r.mu.Lock()
	r.hosts = hosts
	r.defaultTree = defaultTree
	r.regexRoutes = regexRoutes
	r.mu.Unlock()
	return nil
}

func newMethodTree() *MethodTree {
	return &MethodTree{
		trees: make(map[string]*radixNode),
		any:   &radixNode{},
	}
}

func (t *MethodTree) insert(method, path string, route *config.Route, service *config.Service) {
	root := t.rootFor(method, true)
	insertPath(root, path, route, service)
}

func (t *MethodTree) match(method, path string) (*config.Route, *config.Service, map[string]string, bool) {
	segments := splitPath(path)
	if root := t.rootFor(method, false); root != nil {
		if route, service, params, ok := searchPath(root, segments, 0, map[string]string{}); ok {
			return route, service, params, true
		}
	}
	if t.any != nil {
		if route, service, params, ok := searchPath(t.any, segments, 0, map[string]string{}); ok {
			return route, service, params, true
		}
	}
	return nil, nil, nil, false
}

func (t *MethodTree) rootFor(method string, create bool) *radixNode {
	m := strings.ToUpper(strings.TrimSpace(method))
	if m == "*" {
		if t.any == nil && create {
			t.any = &radixNode{}
		}
		return t.any
	}
	if m == "" {
		m = http.MethodGet
	}

	if root, ok := t.trees[m]; ok {
		return root
	}
	if !create {
		return nil
	}
	root := &radixNode{}
	t.trees[m] = root
	return root
}

func insertPath(root *radixNode, path string, route *config.Route, service *config.Service) {
	segments := splitPath(path)
	node := root
	for _, seg := range segments {
		node = node.findOrCreate(seg)
	}
	// Keep the first entry because routes are inserted in pre-sorted priority order.
	if node.route == nil {
		node.route = route
		node.service = service
	}
}

func searchPath(node *radixNode, segments []string, idx int, params map[string]string) (*config.Route, *config.Service, map[string]string, bool) {
	if node == nil {
		return nil, nil, nil, false
	}

	if idx >= len(segments) {
		if node.route != nil {
			return node.route, node.service, cloneParams(params), true
		}
		for _, child := range node.children {
			if child.isWild && child.route != nil {
				return child.route, child.service, cloneParams(params), true
			}
		}
		return nil, nil, nil, false
	}

	current := segments[idx]

	for _, child := range node.children {
		if !child.isWild && child.paramKey == "" && child.path == current {
			if route, service, outParams, ok := searchPath(child, segments, idx+1, params); ok {
				return route, service, outParams, true
			}
		}
	}

	for _, child := range node.children {
		if child.paramKey == "" {
			continue
		}
		nextParams := cloneParams(params)
		nextParams[child.paramKey] = current
		if route, service, outParams, ok := searchPath(child, segments, idx+1, nextParams); ok {
			return route, service, outParams, true
		}
	}

	for _, child := range node.children {
		if !child.isWild {
			continue
		}
		if route, service, outParams, ok := searchWildcard(child, segments, idx, params); ok {
			return route, service, outParams, true
		}
	}

	return nil, nil, nil, false
}

func searchWildcard(wildcardNode *radixNode, segments []string, idx int, params map[string]string) (*config.Route, *config.Service, map[string]string, bool) {
	if wildcardNode.route != nil {
		return wildcardNode.route, wildcardNode.service, cloneParams(params), true
	}
	for consume := idx; consume <= len(segments); consume++ {
		if route, service, outParams, ok := searchPath(wildcardNode, segments, consume, params); ok {
			return route, service, outParams, true
		}
	}
	return nil, nil, nil, false
}

func (n *radixNode) findOrCreate(segment string) *radixNode {
	for _, child := range n.children {
		switch {
		case segment == "*" && child.isWild:
			return child
		case strings.HasPrefix(segment, ":") && child.paramKey != "":
			return child
		case !strings.HasPrefix(segment, ":") && segment != "*" && !child.isWild && child.paramKey == "" && child.path == segment:
			return child
		}
	}

	child := &radixNode{}
	switch {
	case segment == "*":
		child.path = "*"
		child.isWild = true
	case strings.HasPrefix(segment, ":") && len(segment) > 1:
		child.path = segment
		child.paramKey = segment[1:]
	default:
		child.path = segment
	}
	n.children = append(n.children, child)
	return child
}

func setRequestParams(req *http.Request, params map[string]string) {
	ctx := context.WithValue(req.Context(), paramsKey{}, cloneParams(params))
	*req = *req.WithContext(ctx)
}

func applyStripPath(req *http.Request, route *config.Route, originalPath string) {
	if req == nil || route == nil || !route.StripPath {
		return
	}

	prefix, ok := bestStripPrefix(route.Paths, originalPath)
	if !ok || prefix == "/" {
		return
	}

	rewritten := strings.TrimPrefix(normalizePath(originalPath), prefix)
	if rewritten == "" {
		rewritten = "/"
	}
	if !strings.HasPrefix(rewritten, "/") {
		rewritten = "/" + rewritten
	}
	req.URL.Path = rewritten
}

func bestStripPrefix(patterns []string, requestPath string) (string, bool) {
	requestPath = normalizePath(requestPath)
	longest := ""

	for _, pattern := range patterns {
		p := normalizePath(pattern)
		if classifyPath(p) == pathKindRegex {
			continue
		}
		prefix, ok := consumedPrefixFromPattern(p, requestPath)
		if !ok {
			continue
		}
		if len(prefix) > len(longest) {
			longest = prefix
		}
	}

	if longest == "" {
		return "", false
	}
	return longest, true
}

func consumedPrefixFromPattern(pattern, requestPath string) (string, bool) {
	pSeg := splitPath(pattern)
	rSeg := splitPath(requestPath)

	if len(pSeg) == 0 {
		if len(rSeg) == 0 {
			return "/", true
		}
		return "", false
	}

	consumed := make([]string, 0, len(rSeg))
	ri := 0
	for _, ps := range pSeg {
		if ps == "*" {
			if len(consumed) == 0 {
				return "/", true
			}
			return "/" + strings.Join(consumed, "/"), true
		}
		if ri >= len(rSeg) {
			return "", false
		}
		rs := rSeg[ri]

		if strings.HasPrefix(ps, ":") {
			consumed = append(consumed, rs)
			ri++
			continue
		}
		if ps != rs {
			return "", false
		}
		consumed = append(consumed, rs)
		ri++
	}

	if ri != len(rSeg) {
		return "", false
	}
	if len(consumed) == 0 {
		return "/", true
	}
	return "/" + strings.Join(consumed, "/"), true
}

func normalizeMethods(in []string) []string {
	if len(in) == 0 {
		return []string{http.MethodGet}
	}

	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, method := range in {
		m := strings.ToUpper(strings.TrimSpace(method))
		if m == "" {
			continue
		}
		if _, ok := seen[m]; ok {
			continue
		}
		seen[m] = struct{}{}
		out = append(out, m)
	}
	if len(out) == 0 {
		return []string{http.MethodGet}
	}
	return out
}

func normalizeHosts(in []string) []string {
	if len(in) == 0 {
		return []string{""}
	}

	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, host := range in {
		h := normalizeHost(host)
		if _, ok := seen[h]; ok {
			continue
		}
		seen[h] = struct{}{}
		out = append(out, h)
	}
	if len(out) == 0 {
		return []string{""}
	}
	return out
}

func normalizeHost(host string) string {
	h := strings.ToLower(strings.TrimSpace(host))
	h = strings.TrimPrefix(h, "http://")
	h = strings.TrimPrefix(h, "https://")

	if strings.Count(h, ":") == 1 {
		if idx := strings.LastIndexByte(h, ':'); idx > 0 {
			h = h[:idx]
		}
	}
	h = strings.Trim(h, "[]")
	return h
}

func normalizePath(path string) string {
	if strings.TrimSpace(path) == "" {
		return "/"
	}
	p := path
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	p = stdpath.Clean(p)
	if p == "." || p == "" {
		return "/"
	}
	return p
}

func splitPath(path string) []string {
	p := normalizePath(path)
	if p == "/" {
		return nil
	}
	return strings.Split(strings.Trim(p, "/"), "/")
}

func classifyPath(path string) pathKind {
	segments := splitPath(path)
	for _, seg := range segments {
		if seg == "*" || strings.HasPrefix(seg, ":") {
			return pathKindPrefix
		}
	}
	if hasRegexMeta(path) {
		return pathKindRegex
	}
	return pathKindExact
}

func hasRegexMeta(path string) bool {
	return strings.ContainsAny(path, "[](){}+?|^$\\*")
}

func compileRegex(path string) (*regexp.Regexp, error) {
	if len(path) > maxRegexLength {
		return nil, fmt.Errorf("route pattern exceeds maximum length of %d characters", maxRegexLength)
	}
	p := path
	if !strings.HasPrefix(p, "^") {
		p = "^" + p
	}
	if !strings.HasSuffix(p, "$") {
		p += "$"
	}
	return regexp.Compile(p)
}

func methodsToMap(methods []string) map[string]struct{} {
	out := make(map[string]struct{}, len(methods))
	for _, method := range methods {
		out[strings.ToUpper(strings.TrimSpace(method))] = struct{}{}
	}
	return out
}

func cloneParams(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
