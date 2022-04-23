package runtime

import (
	"google.golang.org/grpc"

	options "github.com/binchencoder/janus-gateway/httpoptions"
	vexpb "github.com/binchencoder/gateway-proto/data"
	skypb "github.com/binchencoder/skylb-api/proto"
)

var (
	// CallerServiceId sets the gRPC caller service ID of the gateway.
	// For janus-gateway, it's ServiceId_EASE_GATEWAY.
	CallerServiceId = vexpb.ServiceId_EASE_GATEWAY
)

// Method represents a gRPC service method.
type Method struct {
	Name               string
	Path               string
	HttpMethod         string
	Enabled            bool
	LoginRequired      bool
	ClientSignRequired bool
	IsThirdParty       bool
	SpecifiedSource    options.SpecSourceType
	ApiSource          options.ApiSourceType
	TokenType          options.AuthTokenType
	Timeout            string
}

// Service is the controller class for each grpc service handler.
type Service struct {
	Spec     skypb.ServiceSpec
	Name     string
	Methods  []*Method
	Register func(*ServeMux) error
	Enable   func(spec *skypb.ServiceSpec, conn *grpc.ClientConn)
	Disable  func()
}

// ServiceGroup groups services with the same spec.
type ServiceGroup struct {
	Spec     skypb.ServiceSpec
	Enable   func()
	Disable  func()
	Services map[string]*Service
}

var (
	availableServiceGroups = make(map[string]*ServiceGroup)
)

// AddMethod adds an API method to the service object with the given spec.
func AddMethod(spec *skypb.ServiceSpec, svcName, methodName, path, httpMethod string, loginRequired, clientSignRequired, isThirdParty bool, specSource, apiSource, tokenType, timeout string) {
	sg := availableServiceGroups[spec.String()]
	svc := sg.Services[svcName]
	m := Method{
		Name:               methodName,
		Path:               path,
		HttpMethod:         httpMethod,
		LoginRequired:      loginRequired,
		ClientSignRequired: clientSignRequired,
		IsThirdParty:       isThirdParty,
		SpecifiedSource:    options.SpecSourceType(options.SpecSourceType_value[specSource]),
		ApiSource:          options.ApiSourceType(options.ApiSourceType_value[apiSource]),
		TokenType:          options.AuthTokenType(options.AuthTokenType_value[tokenType]),
		Timeout:            timeout,
	}
	svc.Methods = append(svc.Methods, &m)
}

// AddService adds a service handler to the pool as available list.
// This will not automatically call Regsiter.
func AddService(s *Service, enabler, disabler func()) {
	spec := s.Spec
	sg, ok := availableServiceGroups[spec.String()]
	if !ok {
		sg = &ServiceGroup{
			Spec:     spec,
			Services: map[string]*Service{},
		}
		availableServiceGroups[spec.String()] = sg
	}
	if enabler != nil {
		sg.Enable = enabler
	}
	if disabler != nil {
		sg.Disable = disabler
	}
	sg.Services[s.Name] = s
}

// GetServicGroups returns the current available service groups.
func GetServicGroups() map[string]*ServiceGroup {
	return availableServiceGroups
}

// GetServiceGroup returns the ServiceGroup with the given spec.
func GetServiceGroup(spec *skypb.ServiceSpec) *ServiceGroup {
	return availableServiceGroups[spec.String()]
}
