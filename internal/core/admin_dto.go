package core

import (
	"sort"

	"yuelaiengine/gateway/internal/config"
)

type adminRouteDTO struct {
	PathPrefix       string                 `json:"path_prefix,omitempty"`
	Path             string                 `json:"path,omitempty"`
	ServiceName      string                 `json:"service_name"`
	Plugins          []config.PluginSpec    `json:"plugins,omitempty"`
	Methods          []string               `json:"methods,omitempty"`
	RequiresAuth     bool                   `json:"requires_auth,omitempty"`
	HealthCheckScope string                 `json:"health_check_scope,omitempty"`
	UpstreamProtocol string                 `json:"upstream_protocol,omitempty"`
	ProtocolConvert  string                 `json:"protocol_convert,omitempty"`
	GRPCMethod       string                 `json:"grpc_method,omitempty"`
	ProtoDescriptor  string                 `json:"proto_descriptor_path,omitempty"`
	EmitUnpopulated  bool                   `json:"emit_unpopulated,omitempty"`
	UseProtoNames    bool                   `json:"use_proto_names,omitempty"`
	DiscardUnknown   bool                   `json:"discard_unknown,omitempty"`
	HashOn           string                 `json:"hash_on,omitempty"`
	ABHeader         string                 `json:"ab_header,omitempty"`
	ABVariants       map[string]string      `json:"ab_variants,omitempty"`
	TrafficWeights   map[string]int         `json:"traffic_weights,omitempty"`
	Extra            map[string]interface{} `json:"extra,omitempty"`
}

func routeDTOFromConfig(item *config.RouteConfig) adminRouteDTO {
	if item == nil {
		return adminRouteDTO{}
	}

	dto := adminRouteDTO{
		PathPrefix:       item.PathPrefix,
		Path:             item.Path,
		ServiceName:      item.ServiceName,
		Plugins:          clonePluginSpecs(item.Plugins),
		Methods:          append([]string(nil), item.Methods...),
		RequiresAuth:     item.RequiresAuth,
		HealthCheckScope: item.HealthCheckScope,
		UpstreamProtocol: item.UpstreamProtocol,
		ProtocolConvert:  item.ProtocolConvert,
		GRPCMethod:       item.GRPCMethod,
		ProtoDescriptor:  item.ProtoDescriptor,
		EmitUnpopulated:  item.EmitUnpopulated,
		UseProtoNames:    item.UseProtoNames,
		DiscardUnknown:   item.DiscardUnknown,
		HashOn:           item.HashOn,
		ABHeader:         item.ABHeader,
	}

	if len(item.ABVariants) > 0 {
		dto.ABVariants = make(map[string]string, len(item.ABVariants))
		for k, v := range item.ABVariants {
			dto.ABVariants[k] = v
		}
	}
	if len(item.TrafficWeights) > 0 {
		dto.TrafficWeights = make(map[string]int, len(item.TrafficWeights))
		for k, v := range item.TrafficWeights {
			dto.TrafficWeights[k] = v
		}
	}
	return dto
}

func routeDTOListFromConfig(routes []*config.RouteConfig) []adminRouteDTO {
	out := make([]adminRouteDTO, 0, len(routes))
	for _, item := range routes {
		if item == nil {
			continue
		}
		out = append(out, routeDTOFromConfig(item))
	}
	return out
}

func (d adminRouteDTO) toConfigRoute() config.RouteConfig {
	out := config.RouteConfig{
		PathPrefix:       d.PathPrefix,
		Path:             d.Path,
		ServiceName:      d.ServiceName,
		Plugins:          clonePluginSpecs(d.Plugins),
		Methods:          append([]string(nil), d.Methods...),
		RequiresAuth:     d.RequiresAuth,
		HealthCheckScope: d.HealthCheckScope,
		UpstreamProtocol: d.UpstreamProtocol,
		ProtocolConvert:  d.ProtocolConvert,
		GRPCMethod:       d.GRPCMethod,
		ProtoDescriptor:  d.ProtoDescriptor,
		EmitUnpopulated:  d.EmitUnpopulated,
		UseProtoNames:    d.UseProtoNames,
		DiscardUnknown:   d.DiscardUnknown,
		HashOn:           d.HashOn,
		ABHeader:         d.ABHeader,
	}
	if len(d.ABVariants) > 0 {
		out.ABVariants = make(map[string]string, len(d.ABVariants))
		for k, v := range d.ABVariants {
			out.ABVariants[k] = v
		}
	}
	if len(d.TrafficWeights) > 0 {
		out.TrafficWeights = make(map[string]int, len(d.TrafficWeights))
		for k, v := range d.TrafficWeights {
			out.TrafficWeights[k] = v
		}
	}
	return out
}

type adminTokenBucketDTO struct {
	Capacity   int `json:"capacity"`
	RefillRate int `json:"refill_rate"`
}

type adminServiceDTO struct {
	Name            string                  `json:"name"`
	Instances       []config.InstanceConfig `json:"instances,omitempty"`
	HealthCheckPath string                  `json:"health_check_path,omitempty"`
	LoadBalancer    string                  `json:"load_balancer,omitempty"`
}

func serviceDTOFromConfig(item config.ServiceConfig) adminServiceDTO {
	out := adminServiceDTO{
		Name:            item.Name,
		HealthCheckPath: item.HealthCheckPath,
		LoadBalancer:    item.LoadBalancer,
	}
	if len(item.Instances) > 0 {
		out.Instances = append([]config.InstanceConfig(nil), item.Instances...)
	}
	return out
}

func serviceDTOListFromConfig(items map[string]config.ServiceConfig) []adminServiceDTO {
	if len(items) == 0 {
		return []adminServiceDTO{}
	}
	names := make([]string, 0, len(items))
	for name := range items {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]adminServiceDTO, 0, len(names))
	for _, name := range names {
		out = append(out, serviceDTOFromConfig(items[name]))
	}
	return out
}

func (d adminServiceDTO) toConfigService() config.ServiceConfig {
	out := config.ServiceConfig{
		Name:            d.Name,
		HealthCheckPath: d.HealthCheckPath,
		LoadBalancer:    d.LoadBalancer,
	}
	if len(d.Instances) > 0 {
		out.Instances = append([]config.InstanceConfig(nil), d.Instances...)
	}
	return out
}

type adminRateLimitRuleDTO struct {
	Name        string              `json:"name"`
	Type        string              `json:"type"`
	TokenBucket adminTokenBucketDTO `json:"token_bucket"`
}

func rateLimitRuleDTOFromConfig(item config.RateLimiterRule) adminRateLimitRuleDTO {
	return adminRateLimitRuleDTO{
		Name: item.Name,
		Type: item.Type,
		TokenBucket: adminTokenBucketDTO{
			Capacity:   item.TokenBucket.Capacity,
			RefillRate: item.TokenBucket.RefillRate,
		},
	}
}

func rateLimitRuleDTOListFromConfig(items []config.RateLimiterRule) []adminRateLimitRuleDTO {
	out := make([]adminRateLimitRuleDTO, 0, len(items))
	for _, item := range items {
		out = append(out, rateLimitRuleDTOFromConfig(item))
	}
	return out
}

type adminVersionMetaDTO struct {
	Version   string `json:"version"`
	Source    string `json:"source"`
	CreatedAt string `json:"created_at"`
}

func versionMetaDTO(item *configVersionMeta) *adminVersionMetaDTO {
	if item == nil {
		return nil
	}
	return &adminVersionMetaDTO{
		Version:   item.Version,
		Source:    item.Source,
		CreatedAt: item.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func versionMetaDTOList(items []configVersionMeta) []adminVersionMetaDTO {
	out := make([]adminVersionMetaDTO, 0, len(items))
	for _, item := range items {
		copied := item
		dto := versionMetaDTO(&copied)
		if dto != nil {
			out = append(out, *dto)
		}
	}
	return out
}
