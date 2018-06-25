package basic

import (
	"errors"
	"gitlab.alipay-inc.com/afe/mosn/pkg/api/v2"
	"gitlab.alipay-inc.com/afe/mosn/pkg/log"
	"gitlab.alipay-inc.com/afe/mosn/pkg/protocol"
	"gitlab.alipay-inc.com/afe/mosn/pkg/router"
	"gitlab.alipay-inc.com/afe/mosn/pkg/types"
	"sync"
	"time"
)

func init() {
	router.RegisteRouterConfigFactory(protocol.SofaRpc, NewRouters)
	router.RegisteRouterConfigFactory(protocol.Http2, NewRouters)
	router.RegisteRouterConfigFactory(protocol.Http1, NewRouters)
}

// types.Routers
type Routers struct {
	rMutex  *sync.RWMutex
	routers []router.RouteBase
}

//Routing, use static router first， then use dynamic router if support
func (rc *Routers) Route(headers map[string]string) (types.Route, string) {
	//use static router first, then use dynamic router
	for _, r := range rc.routers {
		if rule, key := r.Match(headers); rule != nil {
			return rule, key
		}
	}
	
	return nil, ""
}

func (rc *Routers) AddRouter(routerName string) {
	
	rc.rMutex.Lock()
	defer rc.rMutex.Unlock()
	
	for _, r := range rc.routers {
		if r.GetRouterName() == routerName {
			log.DefaultLogger.Debugf("[Basic Router]router already exist %s",routerName)
			
			return
		}
	}
	
	// new dynamic router
	// note: as cluster's name is ended with "@DEFAULT" @boqin ...check
	br := &basicRouter{
		name:    routerName,
		service: routerName,
		cluster: routerName,
	}

	if len(rc.routers) > 0 {
		br.globalTimeout = rc.routers[0].GlobalTimeout()
		br.policy = rc.routers[0].Policy().(*routerPolicy)
	} else {
		br.globalTimeout = types.GlobalTimeout
		br.policy = &routerPolicy{
			retryOn:      false,
			retryTimeout: 0,
			numRetries:   0,
		}
	}
	
	rc.routers = append(rc.routers, br)
	log.DefaultLogger.Debugf("[Basic Router]add routes,router name is %s, router %+v",br.name,br)
}

func (rc *Routers) DelRouter(routerName string) {
	rc.rMutex.Lock()
	defer rc.rMutex.Unlock()
	
	for i, r := range rc.routers {
		if r.GetRouterName() == routerName {
			//return
			rc.routers = append(rc.routers[:i],rc.routers[i+1:]...)
			log.DefaultLogger.Debugf("[Basic Router]delete routes,router name %s, routers is",routerName,rc)
			
			return
		}
	}
}

// types.Route
// types.RouteRule
// router.Matchable
type basicRouter struct {
	RouteRuleImplAdaptor
	name          string
	service       string
	cluster       string
	globalTimeout time.Duration
	policy        *routerPolicy
}

type DynamicRouter struct {
	RouteRuleImplAdaptor
	globalTimeout time.Duration
	policy        *routerPolicy
}

func NewRouters(config interface{}) (types.Routers, error) {
	if config, ok := config.(*v2.Proxy); ok {
		routers := make([]router.RouteBase, 0)
		
		for _, r := range config.Routes {
			router := &basicRouter{
				name:          r.Name,
				service:       r.Service,
				cluster:       r.Cluster,
				globalTimeout: r.GlobalTimeout,
			}

			if r.RetryPolicy != nil {
				router.policy = &routerPolicy{
					retryOn:      r.RetryPolicy.RetryOn,
					retryTimeout: r.RetryPolicy.RetryTimeout,
					numRetries:   r.RetryPolicy.NumRetries,
				}
			} else {
				// default
				router.policy = &routerPolicy{
					retryOn:      false,
					retryTimeout: 0,
					numRetries:   0,
				}
			}

			routers = append(routers, router)
		}
		
		if config.SupportDynamicRoute {
			dynamicRouter := DynamicRouter{}
			
			if len(routers) > 0 {
				dynamicRouter.globalTimeout = routers[0].GlobalTimeout()
				dynamicRouter.policy = routers[0].Policy().(*routerPolicy)
			} else {
				dynamicRouter.globalTimeout = types.GlobalTimeout
				dynamicRouter.policy = &routerPolicy{
					retryOn:      false,
					retryTimeout: 0,
					numRetries:   0,
				}
			}
			routers = append(routers, &dynamicRouter)
			log.DefaultLogger.Debugf("Add dynamic router in routers")
		}
		rc := &Routers{
			//new(sync.RWMutex),
			routers:routers,
			
		}
		
		//router.RoutersManager.AddRoutersSet(rc)
		return rc, nil
		
	} else {
		return nil, errors.New("invalid config struct")
	}
}

func (srr *basicRouter) Match(headers map[string]string) (types.Route,string ){
	if headers == nil {
		return nil, ""
	}
	
	var ok bool
	var service string
	
	if service, ok = headers["Service"]; !ok {
		if service, ok = headers["service"]; !ok {
			return nil, ""
		}
	}
	
	if srr.service == service {
		return srr, service
	} else {
		return nil, service
	}
	return srr, service
}

func (drr *DynamicRouter) Match(headers map[string]string) (types.Route, string) {
	if headers == nil {
		return nil, ""
	}
	
	var ok bool
	var service string
	
	if service, ok = headers["Service"]; !ok {
		if service, ok = headers["service"]; !ok {
			return nil, ""
		}
	}
	log.DefaultLogger.Debugf("match dynamic router in routers")
	return drr, service
}

func (srr *basicRouter) RedirectRule() types.RedirectRule {
	return nil
}

func (drr *DynamicRouter) RedirectRule() types.RedirectRule {
	return nil
}

func (srr *basicRouter) RouteRule() types.RouteRule {
	return srr
}

func (drr *DynamicRouter) RouteRule() types.RouteRule {
	return drr
}

func (srr *basicRouter) TraceDecorator() types.TraceDecorator {
	return nil
}

func (drr *DynamicRouter) TraceDecorator() types.TraceDecorator {
	return nil
}


func (srr *basicRouter) ClusterName(clusterKey string) string {
	return srr.cluster
}

//use servicename
func (drr *DynamicRouter) ClusterName(clusterKey string) string {
	return drr.MapRule(clusterKey)
}


func (srr *basicRouter) GlobalTimeout() time.Duration {
	return srr.globalTimeout
}

func (drr *DynamicRouter) GlobalTimeout() time.Duration {
	return drr.globalTimeout
}

func (srr *basicRouter) Policy() types.Policy {
	return srr.policy
}

func (drr *DynamicRouter) Policy() types.Policy {
	return drr.policy
}

//For dynamic use echo at this time
func (drr *DynamicRouter)MapRule(routeKey string) string{
	return routeKey
}

func (srr *basicRouter) GetRouterName() string {
	return srr.name
}

func (drr *DynamicRouter) GetRouterName() string {
	return ""
}

type routerPolicy struct {
	retryOn      bool
	retryTimeout time.Duration
	numRetries   int
}

func (p *routerPolicy) RetryOn() bool {
	return p.retryOn
}

func (p *routerPolicy) TryTimeout() time.Duration {
	return p.retryTimeout
}

func (p *routerPolicy) NumRetries() int {
	return p.numRetries
}

func (p *routerPolicy) RetryPolicy() types.RetryPolicy {
	return p
}

func (p *routerPolicy) ShadowPolicy() types.ShadowPolicy {
	return nil
}

func (p *routerPolicy) CorsPolicy() types.CorsPolicy {
	return nil
}

func (p *routerPolicy) LoadBalancerPolicy() types.LoadBalancerPolicy {
	return nil
}
