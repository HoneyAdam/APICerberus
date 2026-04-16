package admin

import (
	"errors"
	"net/http"
	"strings"

	"github.com/APICerberus/APICerebrus/internal/config"
	coerce "github.com/APICerberus/APICerebrus/internal/pkg/coerce"
	jsonutil "github.com/APICerberus/APICerebrus/internal/pkg/json"
	"github.com/APICerberus/APICerebrus/internal/pkg/uuid"
	"github.com/APICerberus/APICerebrus/internal/store"
	"github.com/graphql-go/graphql"
)

// GraphQLHandler handles GraphQL requests for the admin API
type GraphQLHandler struct {
	schema graphql.Schema
	server *Server
}

// NewGraphQLHandler creates a new GraphQL handler with the complete schema
func NewGraphQLHandler(server *Server) (*GraphQLHandler, error) {
	h := &GraphQLHandler{server: server}

	schema, err := graphql.NewSchema(graphql.SchemaConfig{
		Query:    h.buildQueryType(),
		Mutation: h.buildMutationType(),
	})
	if err != nil {
		return nil, err
	}

	h.schema = schema
	return h, nil
}

// ServeHTTP implements http.Handler
func (h *GraphQLHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Query         string         `json:"query"`
		Variables     map[string]any `json:"variables"`
		OperationName string         `json:"operationName"`
	}

	if err := jsonutil.ReadJSON(r, &req, 1<<20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	result := graphql.Do(graphql.Params{
		Schema: h.schema,
		RequestString: req.Query,
		VariableValues: req.Variables,
		OperationName: req.OperationName,
		Context: r.Context(),
	})

	// F-012: Block introspection queries when disabled (default).
	h.server.mu.RLock()
	introspectionEnabled := h.server.cfg.Admin.GraphQLIntrospection
	h.server.mu.RUnlock()
	if !introspectionEnabled && isIntrospectionQuery(req.Query) {
		_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
			"errors": []map[string]any{{
				"message": "introspection is disabled",
			}},
			"data": nil,
		})
		return
	}

	if len(result.Errors) > 0 {
		_ = jsonutil.WriteJSON(w, http.StatusOK, map[string]any{
			"errors": result.Errors,
			"data":   result.Data,
		})
		return
	}

	_ = jsonutil.WriteJSON(w, http.StatusOK, result)
}

// buildQueryType builds the GraphQL query type
func (h *GraphQLHandler) buildQueryType() *graphql.Object {
	return graphql.NewObject(graphql.ObjectConfig{
		Name: "Query",
		Fields: graphql.Fields{
			"services": &graphql.Field{
				Type: graphql.NewList(serviceType),
				Resolve: func(p graphql.ResolveParams) (any, error) {
					h.server.mu.RLock()
					defer h.server.mu.RUnlock()
					return h.server.cfg.Services, nil
				},
			},
			"service": &graphql.Field{
				Type: serviceType,
				Args: graphql.FieldConfigArgument{
					"id": &graphql.ArgumentConfig{Type: graphql.String},
				},
				Resolve: func(p graphql.ResolveParams) (any, error) {
					id, _ := p.Args["id"].(string)
					h.server.mu.RLock()
					defer h.server.mu.RUnlock()
					return serviceByID(h.server.cfg, id), nil
				},
			},
			"routes": &graphql.Field{
				Type: graphql.NewList(routeType),
				Resolve: func(p graphql.ResolveParams) (any, error) {
					h.server.mu.RLock()
					defer h.server.mu.RUnlock()
					return h.server.cfg.Routes, nil
				},
			},
			"route": &graphql.Field{
				Type: routeType,
				Args: graphql.FieldConfigArgument{
					"id": &graphql.ArgumentConfig{Type: graphql.String},
				},
				Resolve: func(p graphql.ResolveParams) (any, error) {
					id, _ := p.Args["id"].(string)
					h.server.mu.RLock()
					defer h.server.mu.RUnlock()
					return routeByID(h.server.cfg, id), nil
				},
			},
			"upstreams": &graphql.Field{
				Type: graphql.NewList(upstreamType),
				Resolve: func(p graphql.ResolveParams) (any, error) {
					h.server.mu.RLock()
					defer h.server.mu.RUnlock()
					return h.server.cfg.Upstreams, nil
				},
			},
			"upstream": &graphql.Field{
				Type: upstreamType,
				Args: graphql.FieldConfigArgument{
					"id": &graphql.ArgumentConfig{Type: graphql.String},
				},
				Resolve: func(p graphql.ResolveParams) (any, error) {
					id, _ := p.Args["id"].(string)
					h.server.mu.RLock()
					defer h.server.mu.RUnlock()
					return upstreamByID(h.server.cfg, id), nil
				},
			},
			"consumers": &graphql.Field{
				Type: graphql.NewList(consumerType),
				Resolve: func(p graphql.ResolveParams) (any, error) {
					h.server.mu.RLock()
					defer h.server.mu.RUnlock()
					return h.server.cfg.Consumers, nil
				},
			},
			"consumer": &graphql.Field{
				Type: consumerType,
				Args: graphql.FieldConfigArgument{
					"id": &graphql.ArgumentConfig{Type: graphql.String},
				},
				Resolve: func(p graphql.ResolveParams) (any, error) {
					id, _ := p.Args["id"].(string)
					h.server.mu.RLock()
					defer h.server.mu.RUnlock()
					for i := range h.server.cfg.Consumers {
						if h.server.cfg.Consumers[i].ID == id {
							return &h.server.cfg.Consumers[i], nil
						}
					}
					return nil, nil
				},
			},
			"users": &graphql.Field{
				Type: graphql.NewList(graphQLUserType),
				Resolve: func(p graphql.ResolveParams) (any, error) {
					st := h.server.gateway.Store()
					if st == nil {
						return nil, errors.New("store not available")
					}
					result, err := st.Users().List(store.UserListOptions{Limit: 1000})
					if err != nil {
						return nil, err
					}
					return result.Users, nil
				},
			},
			"user": &graphql.Field{
				Type: graphQLUserType,
				Args: graphql.FieldConfigArgument{
					"id": &graphql.ArgumentConfig{Type: graphql.String},
				},
				Resolve: func(p graphql.ResolveParams) (any, error) {
					id, _ := p.Args["id"].(string)
					st := h.server.gateway.Store()
					if st == nil {
						return nil, errors.New("store not available")
					}
					return st.Users().FindByID(id)
				},
			},
			"auditLogs": &graphql.Field{
				Type: auditLogConnectionType,
				Args: graphql.FieldConfigArgument{
					"limit":  &graphql.ArgumentConfig{Type: graphql.Int, DefaultValue: 20},
					"offset": &graphql.ArgumentConfig{Type: graphql.Int, DefaultValue: 0},
					"userId": &graphql.ArgumentConfig{Type: graphql.String},
					"route":  &graphql.ArgumentConfig{Type: graphql.String},
				},
				Resolve: func(p graphql.ResolveParams) (any, error) {
					st := h.server.gateway.Store()
					if st == nil {
						return nil, errors.New("store not available")
					}

					limit, _ := p.Args["limit"].(int)
					offset, _ := p.Args["offset"].(int)
					userID, _ := p.Args["userId"].(string)
					route, _ := p.Args["route"].(string)

					filters := store.AuditSearchFilters{
						UserID: userID,
						Route:  route,
						Limit:  limit,
						Offset: offset,
					}

					result, err := st.Audits().Search(filters)
					if err != nil {
						return nil, err
					}

					return map[string]any{
						"entries": result.Entries,
						"total":   result.Total,
					}, nil
				},
			},
			"gatewayInfo": &graphql.Field{
				Type: gatewayInfoType,
				Resolve: func(p graphql.ResolveParams) (any, error) {
					h.server.mu.RLock()
					defer h.server.mu.RUnlock()
					return map[string]any{
						"services":  len(h.server.cfg.Services),
						"routes":    len(h.server.cfg.Routes),
						"upstreams": len(h.server.cfg.Upstreams),
						"consumers": len(h.server.cfg.Consumers),
					}, nil
				},
			},
		},
	})
}

// isIntrospectionQuery returns true if the query is a GraphQL introspection query.
// Introspection queries use __schema or __type introspection fields.
func isIntrospectionQuery(query string) bool {
	return strings.Contains(query, "__schema") || strings.Contains(query, "__type")
}

// buildMutationType builds the GraphQL mutation type
func (h *GraphQLHandler) buildMutationType() *graphql.Object {
	return graphql.NewObject(graphql.ObjectConfig{
		Name: "Mutation",
		Fields: graphql.Fields{
			"createService": &graphql.Field{
				Type: serviceType,
				Args: graphql.FieldConfigArgument{
					"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(serviceInputType)},
				},
				Resolve: h.resolveCreateService,
			},
			"updateService": &graphql.Field{
				Type: serviceType,
				Args: graphql.FieldConfigArgument{
					"id":    &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
					"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(serviceInputType)},
				},
				Resolve: h.resolveUpdateService,
			},
			"deleteService": &graphql.Field{
				Type: graphql.Boolean,
				Args: graphql.FieldConfigArgument{
					"id": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
				},
				Resolve: h.resolveDeleteService,
			},
			"createRoute": &graphql.Field{
				Type: routeType,
				Args: graphql.FieldConfigArgument{
					"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(routeInputType)},
				},
				Resolve: h.resolveCreateRoute,
			},
			"updateRoute": &graphql.Field{
				Type: routeType,
				Args: graphql.FieldConfigArgument{
					"id":    &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
					"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(routeInputType)},
				},
				Resolve: h.resolveUpdateRoute,
			},
			"deleteRoute": &graphql.Field{
				Type: graphql.Boolean,
				Args: graphql.FieldConfigArgument{
					"id": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
				},
				Resolve: h.resolveDeleteRoute,
			},
			"createUpstream": &graphql.Field{
				Type: upstreamType,
				Args: graphql.FieldConfigArgument{
					"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(upstreamInputType)},
				},
				Resolve: h.resolveCreateUpstream,
			},
			"updateUpstream": &graphql.Field{
				Type: upstreamType,
				Args: graphql.FieldConfigArgument{
					"id":    &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
					"input": &graphql.ArgumentConfig{Type: graphql.NewNonNull(upstreamInputType)},
				},
				Resolve: h.resolveUpdateUpstream,
			},
			"deleteUpstream": &graphql.Field{
				Type: graphql.Boolean,
				Args: graphql.FieldConfigArgument{
					"id": &graphql.ArgumentConfig{Type: graphql.NewNonNull(graphql.String)},
				},
				Resolve: h.resolveDeleteUpstream,
			},
		},
	})
}

// GraphQL Types
var serviceType = graphql.NewObject(graphql.ObjectConfig{
	Name: "Service",
	Fields: graphql.Fields{
		"id":             &graphql.Field{Type: graphql.String},
		"name":           &graphql.Field{Type: graphql.String},
		"protocol":       &graphql.Field{Type: graphql.String},
		"upstream":       &graphql.Field{Type: graphql.String},
		"connectTimeout": &graphql.Field{Type: graphql.String},
		"readTimeout":    &graphql.Field{Type: graphql.String},
		"writeTimeout":   &graphql.Field{Type: graphql.String},
	},
})

var routeType = graphql.NewObject(graphql.ObjectConfig{
	Name: "Route",
	Fields: graphql.Fields{
		"id":           &graphql.Field{Type: graphql.String},
		"name":         &graphql.Field{Type: graphql.String},
		"service":      &graphql.Field{Type: graphql.String},
		"hosts":        &graphql.Field{Type: graphql.NewList(graphql.String)},
		"paths":        &graphql.Field{Type: graphql.NewList(graphql.String)},
		"methods":      &graphql.Field{Type: graphql.NewList(graphql.String)},
		"stripPath":    &graphql.Field{Type: graphql.Boolean},
		"preserveHost": &graphql.Field{Type: graphql.Boolean},
		"priority":     &graphql.Field{Type: graphql.Int},
		"plugins":      &graphql.Field{Type: graphql.NewList(pluginConfigType)},
	},
})

var upstreamType = graphql.NewObject(graphql.ObjectConfig{
	Name: "Upstream",
	Fields: graphql.Fields{
		"id":        &graphql.Field{Type: graphql.String},
		"name":      &graphql.Field{Type: graphql.String},
		"algorithm": &graphql.Field{Type: graphql.String},
		"targets":   &graphql.Field{Type: graphql.NewList(upstreamTargetType)},
	},
})

var upstreamTargetType = graphql.NewObject(graphql.ObjectConfig{
	Name: "UpstreamTarget",
	Fields: graphql.Fields{
		"id":      &graphql.Field{Type: graphql.String},
		"address": &graphql.Field{Type: graphql.String},
		"weight":  &graphql.Field{Type: graphql.Int},
	},
})

var consumerType = graphql.NewObject(graphql.ObjectConfig{
	Name: "Consumer",
	Fields: graphql.Fields{
		"id":        &graphql.Field{Type: graphql.String},
		"name":      &graphql.Field{Type: graphql.String},
		"apiKeys":   &graphql.Field{Type: graphql.NewList(consumerAPIKeyType)},
		"aclGroups": &graphql.Field{Type: graphql.NewList(graphql.String)},
	},
})

var consumerAPIKeyType = graphql.NewObject(graphql.ObjectConfig{
	Name: "ConsumerAPIKey",
	Fields: graphql.Fields{
		"id": &graphql.Field{Type: graphql.String},
		"key": &graphql.Field{Type: graphql.String, Resolve: func(p graphql.ResolveParams) (any, error) {
			// Redact raw key values — only show prefix and last 4 chars
			if k, ok := p.Source.(map[string]any); ok {
				if raw, ok := k["key"].(string); ok && len(raw) > 8 {
					return raw[:8] + "..." + raw[len(raw)-4:], nil
				}
			}
			return "***redacted***", nil
		}},
		"createdAt": &graphql.Field{Type: graphql.String},
		"expiresAt": &graphql.Field{Type: graphql.String},
	},
})

var graphQLUserType = graphql.NewObject(graphql.ObjectConfig{
	Name: "User",
	Fields: graphql.Fields{
		"id":        &graphql.Field{Type: graphql.String},
		"email":     &graphql.Field{Type: graphql.String},
		"name":      &graphql.Field{Type: graphql.String},
		"role":      &graphql.Field{Type: graphql.String},
		"active":    &graphql.Field{Type: graphql.Boolean},
		"createdAt": &graphql.Field{Type: graphql.String},
	},
})

var auditLogType = graphql.NewObject(graphql.ObjectConfig{
	Name: "AuditLog",
	Fields: graphql.Fields{
		"id":           &graphql.Field{Type: graphql.String},
		"requestId":    &graphql.Field{Type: graphql.String},
		"routeId":      &graphql.Field{Type: graphql.String},
		"routeName":    &graphql.Field{Type: graphql.String},
		"serviceName":  &graphql.Field{Type: graphql.String},
		"userId":       &graphql.Field{Type: graphql.String},
		"consumerName": &graphql.Field{Type: graphql.String},
		"method":       &graphql.Field{Type: graphql.String},
		"path":         &graphql.Field{Type: graphql.String},
		"statusCode":   &graphql.Field{Type: graphql.Int},
		"latencyMs":    &graphql.Field{Type: graphql.Int},
		"clientIp":     &graphql.Field{Type: graphql.String},
		"blocked":      &graphql.Field{Type: graphql.Boolean},
		"createdAt":    &graphql.Field{Type: graphql.String},
	},
})

var auditLogConnectionType = graphql.NewObject(graphql.ObjectConfig{
	Name: "AuditLogConnection",
	Fields: graphql.Fields{
		"entries": &graphql.Field{Type: graphql.NewList(auditLogType)},
		"total":   &graphql.Field{Type: graphql.Int},
	},
})

var gatewayInfoType = graphql.NewObject(graphql.ObjectConfig{
	Name: "GatewayInfo",
	Fields: graphql.Fields{
		"services":  &graphql.Field{Type: graphql.Int},
		"routes":    &graphql.Field{Type: graphql.Int},
		"upstreams": &graphql.Field{Type: graphql.Int},
		"consumers": &graphql.Field{Type: graphql.Int},
	},
})

var pluginConfigType = graphql.NewObject(graphql.ObjectConfig{
	Name: "PluginConfig",
	Fields: graphql.Fields{
		"name":    &graphql.Field{Type: graphql.String},
		"enabled": &graphql.Field{Type: graphql.Boolean},
	},
})

// Input Types
var serviceInputType = graphql.NewInputObject(graphql.InputObjectConfig{
	Name: "ServiceInput",
	Fields: graphql.InputObjectConfigFieldMap{
		"id":             &graphql.InputObjectFieldConfig{Type: graphql.String},
		"name":           &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
		"protocol":       &graphql.InputObjectFieldConfig{Type: graphql.String, DefaultValue: "http"},
		"upstream":       &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
		"connectTimeout": &graphql.InputObjectFieldConfig{Type: graphql.String, DefaultValue: "5s"},
		"readTimeout":    &graphql.InputObjectFieldConfig{Type: graphql.String, DefaultValue: "30s"},
		"writeTimeout":   &graphql.InputObjectFieldConfig{Type: graphql.String, DefaultValue: "30s"},
	},
})

var routeInputType = graphql.NewInputObject(graphql.InputObjectConfig{
	Name: "RouteInput",
	Fields: graphql.InputObjectConfigFieldMap{
		"id":           &graphql.InputObjectFieldConfig{Type: graphql.String},
		"name":         &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
		"service":      &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
		"hosts":        &graphql.InputObjectFieldConfig{Type: graphql.NewList(graphql.String)},
		"paths":        &graphql.InputObjectFieldConfig{Type: graphql.NewList(graphql.String)},
		"methods":      &graphql.InputObjectFieldConfig{Type: graphql.NewList(graphql.String)},
		"stripPath":    &graphql.InputObjectFieldConfig{Type: graphql.Boolean, DefaultValue: false},
		"preserveHost": &graphql.InputObjectFieldConfig{Type: graphql.Boolean, DefaultValue: false},
		"priority":     &graphql.InputObjectFieldConfig{Type: graphql.Int, DefaultValue: 0},
	},
})

var upstreamInputType = graphql.NewInputObject(graphql.InputObjectConfig{
	Name: "UpstreamInput",
	Fields: graphql.InputObjectConfigFieldMap{
		"id":        &graphql.InputObjectFieldConfig{Type: graphql.String},
		"name":      &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
		"algorithm": &graphql.InputObjectFieldConfig{Type: graphql.String, DefaultValue: "round_robin"},
		"targets":   &graphql.InputObjectFieldConfig{Type: graphql.NewList(upstreamTargetInputType)},
	},
})

var upstreamTargetInputType = graphql.NewInputObject(graphql.InputObjectConfig{
	Name: "UpstreamTargetInput",
	Fields: graphql.InputObjectConfigFieldMap{
		"id":      &graphql.InputObjectFieldConfig{Type: graphql.String},
		"address": &graphql.InputObjectFieldConfig{Type: graphql.NewNonNull(graphql.String)},
		"weight":  &graphql.InputObjectFieldConfig{Type: graphql.Int, DefaultValue: 100},
	},
})

// Mutation Resolvers
func (h *GraphQLHandler) resolveCreateService(p graphql.ResolveParams) (any, error) {
	input, _ := p.Args["input"].(map[string]any)

	svc := config.Service{
		Name:     coerce.GetString(input, "name"),
		Protocol: coerce.GetString(input, "protocol"),
		Upstream: coerce.GetString(input, "upstream"),
	}

	if id := coerce.GetString(input, "id"); id != "" {
		svc.ID = id
	} else {
		id, err := uuid.NewString()
		if err != nil {
			return nil, err
		}
		svc.ID = id
	}

	if err := validateServiceInput(svc); err != nil {
		return nil, err
	}

	if err := h.server.mutateConfig(func(cfg *config.Config) error {
		if serviceByID(cfg, svc.ID) != nil {
			return errors.New("service id already exists")
		}
		if serviceByName(cfg, svc.Name) != nil {
			return errors.New("service name already exists")
		}
		if !upstreamExists(cfg, svc.Upstream) {
			return errors.New("referenced upstream does not exist")
		}
		cfg.Services = append(cfg.Services, svc)
		return nil
	}); err != nil {
		return nil, err
	}

	return svc, nil
}

func (h *GraphQLHandler) resolveUpdateService(p graphql.ResolveParams) (any, error) {
	id, _ := p.Args["id"].(string)
	input, _ := p.Args["input"].(map[string]any)

	svc := config.Service{
		ID:       id,
		Name:     coerce.GetString(input, "name"),
		Protocol: coerce.GetString(input, "protocol"),
		Upstream: coerce.GetString(input, "upstream"),
	}

	if err := validateServiceInput(svc); err != nil {
		return nil, err
	}

	if err := h.server.mutateConfig(func(cfg *config.Config) error {
		idx := serviceIndexByID(cfg, id)
		if idx < 0 {
			return errors.New("service not found")
		}
		if !upstreamExists(cfg, svc.Upstream) {
			return errors.New("referenced upstream does not exist")
		}
		for i := range cfg.Services {
			if i != idx && strings.EqualFold(cfg.Services[i].Name, svc.Name) {
				return errors.New("service name already exists")
			}
		}
		cfg.Services[idx] = svc
		return nil
	}); err != nil {
		return nil, err
	}

	return svc, nil
}

func (h *GraphQLHandler) resolveDeleteService(p graphql.ResolveParams) (any, error) {
	id, _ := p.Args["id"].(string)

	if err := h.server.mutateConfig(func(cfg *config.Config) error {
		idx := serviceIndexByID(cfg, id)
		if idx < 0 {
			return errors.New("service not found")
		}
		svc := cfg.Services[idx]
		for _, rt := range cfg.Routes {
			if rt.Service == svc.ID || rt.Service == svc.Name {
				return errors.New("service is referenced by route")
			}
		}
		cfg.Services = append(cfg.Services[:idx], cfg.Services[idx+1:]...)
		return nil
	}); err != nil {
		return false, err
	}

	return true, nil
}

func (h *GraphQLHandler) resolveCreateRoute(p graphql.ResolveParams) (any, error) {
	input, _ := p.Args["input"].(map[string]any)

	route := config.Route{
		Name:         coerce.GetString(input, "name"),
		Service:      coerce.GetString(input, "service"),
		Hosts:        coerce.GetStringSlice(input, "hosts"),
		Paths:        coerce.GetStringSlice(input, "paths"),
		Methods:      coerce.GetStringSlice(input, "methods"),
		StripPath:    coerce.GetBool(input, "stripPath", false),
		PreserveHost: coerce.GetBool(input, "preserveHost", false),
		Priority:     coerce.GetInt(input, "priority", 0),
	}

	if id := coerce.GetString(input, "id"); id != "" {
		route.ID = id
	} else {
		id, err := uuid.NewString()
		if err != nil {
			return nil, err
		}
		route.ID = id
	}

	if err := validateRouteInput(route); err != nil {
		return nil, err
	}

	if err := h.server.mutateConfig(func(cfg *config.Config) error {
		if routeByID(cfg, route.ID) != nil {
			return errors.New("route id already exists")
		}
		if routeByName(cfg, route.Name) != nil {
			return errors.New("route name already exists")
		}
		if !serviceExists(cfg, route.Service) {
			return errors.New("referenced service does not exist")
		}
		cfg.Routes = append(cfg.Routes, route)
		return nil
	}); err != nil {
		return nil, err
	}

	return route, nil
}

func (h *GraphQLHandler) resolveUpdateRoute(p graphql.ResolveParams) (any, error) {
	id, _ := p.Args["id"].(string)
	input, _ := p.Args["input"].(map[string]any)

	route := config.Route{
		ID:           id,
		Name:         coerce.GetString(input, "name"),
		Service:      coerce.GetString(input, "service"),
		Hosts:        coerce.GetStringSlice(input, "hosts"),
		Paths:        coerce.GetStringSlice(input, "paths"),
		Methods:      coerce.GetStringSlice(input, "methods"),
		StripPath:    coerce.GetBool(input, "stripPath", false),
		PreserveHost: coerce.GetBool(input, "preserveHost", false),
		Priority:     coerce.GetInt(input, "priority", 0),
	}

	if err := validateRouteInput(route); err != nil {
		return nil, err
	}

	if err := h.server.mutateConfig(func(cfg *config.Config) error {
		idx := routeIndexByID(cfg, id)
		if idx < 0 {
			return errors.New("route not found")
		}
		if !serviceExists(cfg, route.Service) {
			return errors.New("referenced service does not exist")
		}
		for i := range cfg.Routes {
			if i != idx && strings.EqualFold(cfg.Routes[i].Name, route.Name) {
				return errors.New("route name already exists")
			}
		}
		cfg.Routes[idx] = route
		return nil
	}); err != nil {
		return nil, err
	}

	return route, nil
}

func (h *GraphQLHandler) resolveDeleteRoute(p graphql.ResolveParams) (any, error) {
	id, _ := p.Args["id"].(string)

	if err := h.server.mutateConfig(func(cfg *config.Config) error {
		idx := routeIndexByID(cfg, id)
		if idx < 0 {
			return errors.New("route not found")
		}
		cfg.Routes = append(cfg.Routes[:idx], cfg.Routes[idx+1:]...)
		return nil
	}); err != nil {
		return false, err
	}

	return true, nil
}

func (h *GraphQLHandler) resolveCreateUpstream(p graphql.ResolveParams) (any, error) {
	input, _ := p.Args["input"].(map[string]any)

	up := config.Upstream{
		Name:      coerce.GetString(input, "name"),
		Algorithm: coerce.GetString(input, "algorithm"),
	}

	if id := coerce.GetString(input, "id"); id != "" {
		up.ID = id
	} else {
		id, err := uuid.NewString()
		if err != nil {
			return nil, err
		}
		up.ID = id
	}

	// Parse targets
	if targetsRaw, ok := input["targets"].([]any); ok {
		for _, t := range targetsRaw {
			if targetMap, ok := t.(map[string]any); ok {
				target := config.UpstreamTarget{
					Address: coerce.GetString(targetMap, "address"),
					Weight:  coerce.GetInt(targetMap, "weight", 0),
				}
				if tid := coerce.GetString(targetMap, "id"); tid != "" {
					target.ID = tid
				} else {
					tid, err := uuid.NewString()
					if err != nil {
						return nil, err
					}
					target.ID = tid
				}
				up.Targets = append(up.Targets, target)
			}
		}
	}

	if err := validateUpstreamInput(up); err != nil {
		return nil, err
	}

	if err := h.server.mutateConfig(func(cfg *config.Config) error {
		if upstreamByID(cfg, up.ID) != nil {
			return errors.New("upstream id already exists")
		}
		if upstreamByName(cfg, up.Name) != nil {
			return errors.New("upstream name already exists")
		}
		cfg.Upstreams = append(cfg.Upstreams, up)
		return nil
	}); err != nil {
		return nil, err
	}

	return up, nil
}

func (h *GraphQLHandler) resolveUpdateUpstream(p graphql.ResolveParams) (any, error) {
	id, _ := p.Args["id"].(string)
	input, _ := p.Args["input"].(map[string]any)

	up := config.Upstream{
		ID:        id,
		Name:      coerce.GetString(input, "name"),
		Algorithm: coerce.GetString(input, "algorithm"),
	}

	// Parse targets
	if targetsRaw, ok := input["targets"].([]any); ok {
		for _, t := range targetsRaw {
			if targetMap, ok := t.(map[string]any); ok {
				target := config.UpstreamTarget{
					ID:      coerce.GetString(targetMap, "id"),
					Address: coerce.GetString(targetMap, "address"),
					Weight:  coerce.GetInt(targetMap, "weight", 0),
				}
				if target.ID == "" {
					tid, err := uuid.NewString()
					if err != nil {
						return nil, err
					}
					target.ID = tid
				}
				up.Targets = append(up.Targets, target)
			}
		}
	}

	if err := validateUpstreamInput(up); err != nil {
		return nil, err
	}

	if err := h.server.mutateConfig(func(cfg *config.Config) error {
		idx := upstreamIndexByID(cfg, id)
		if idx < 0 {
			return errors.New("upstream not found")
		}
		for i := range cfg.Upstreams {
			if i != idx && strings.EqualFold(cfg.Upstreams[i].Name, up.Name) {
				return errors.New("upstream name already exists")
			}
		}
		cfg.Upstreams[idx] = up
		return nil
	}); err != nil {
		return nil, err
	}

	return up, nil
}

func (h *GraphQLHandler) resolveDeleteUpstream(p graphql.ResolveParams) (any, error) {
	id, _ := p.Args["id"].(string)

	if err := h.server.mutateConfig(func(cfg *config.Config) error {
		idx := upstreamIndexByID(cfg, id)
		if idx < 0 {
			return errors.New("upstream not found")
		}
		up := cfg.Upstreams[idx]
		for _, svc := range cfg.Services {
			if svc.Upstream == up.ID || svc.Upstream == up.Name {
				return errors.New("upstream is referenced by service")
			}
		}
		cfg.Upstreams = append(cfg.Upstreams[:idx], cfg.Upstreams[idx+1:]...)
		return nil
	}); err != nil {
		return false, err
	}

	return true, nil
}

// handleGraphQL is the HTTP handler for GraphQL endpoint
func (s *Server) handleGraphQL(w http.ResponseWriter, r *http.Request) {
	handler, err := NewGraphQLHandler(s)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "graphql_init_failed", err.Error())
		return
	}
	handler.ServeHTTP(w, r)
}

// RegisterGraphQLRoutes registers the GraphQL endpoint
func (s *Server) RegisterGraphQLRoutes() {
	s.handle("POST /admin/graphql", s.handleGraphQL)
	s.handle("GET /admin/graphql", s.handleGraphQL)
}
