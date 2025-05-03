package xds

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	matcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/types"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/config"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/k8sclient"
	"github.com/williamhogman/k8s-sched-demo/scheduler/internal/persistence"
	inttypes "github.com/williamhogman/k8s-sched-demo/scheduler/internal/types"
)

// ServiceParams contains dependencies for the XDS service
type ServiceParams struct {
	fx.In

	Config    *config.Config
	Store     persistence.Store
	K8sClient k8sclient.K8sClientInterface
	Logger    *zap.Logger
	Lifecycle fx.Lifecycle
}

// RegisterParams contains dependencies for registering the xDS server with a gRPC server
type RegisterParams struct {
	fx.In

	GRPCServer *grpc.Server `name:"grpcServer"`
	XDSService *Service
}

// RegisterXDS registers the XDS service with a gRPC server
func RegisterXDS(params RegisterParams) {
	discovery.RegisterAggregatedDiscoveryServiceServer(params.GRPCServer, params.XDSService.server)
}

// Service implements the XDS control plane service
type Service struct {
	config        *config.Config
	store         persistence.Store
	k8sClient     k8sclient.K8sClientInterface
	logger        *zap.Logger
	server        server.Server
	snapshotCache cache.SnapshotCache
	mu            sync.RWMutex
	routes        map[inttypes.ProjectID]inttypes.SandboxID
	grpcServer    *grpc.Server
}

// wrapper to adapt zap logger to xDS logger interface
type logWrapper struct {
	logger *zap.Logger
}

func (l logWrapper) Debugf(format string, args ...interface{}) {
	l.logger.Debug(fmt.Sprintf(format, args...))
}

func (l logWrapper) Infof(format string, args ...interface{}) {
	l.logger.Info(fmt.Sprintf(format, args...))
}

func (l logWrapper) Warnf(format string, args ...interface{}) {
	l.logger.Warn(fmt.Sprintf(format, args...))
}

func (l logWrapper) Errorf(format string, args ...interface{}) {
	l.logger.Error(fmt.Sprintf(format, args...))
}

// New creates a new XDS service
func New(params ServiceParams) *Service {
	srv := &Service{
		config:    params.Config,
		store:     params.Store,
		k8sClient: params.K8sClient,
		logger:    params.Logger.Named("xds-service"),
		routes:    make(map[inttypes.ProjectID]inttypes.SandboxID),
	}

	// Create logger wrapper
	loggerWrapper := logWrapper{logger: srv.logger}

	// Create the snapshot cache
	srv.snapshotCache = cache.NewSnapshotCache(false, cache.IDHash{}, loggerWrapper)

	// Create the server
	srv.server = server.NewServer(context.Background(), srv.snapshotCache, nil)

	// Create a dedicated gRPC server for XDS
	srv.grpcServer = grpc.NewServer()
	discovery.RegisterAggregatedDiscoveryServiceServer(srv.grpcServer, srv.server)

	// Register lifecycle hooks
	params.Lifecycle.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			// Load initial routes from store
			if err := srv.loadInitialRoutes(ctx); err != nil {
				srv.logger.Error("Failed to load initial routes", zap.Error(err))
			}

			// Subscribe to project-sandbox mapping updates
			go func() {
				if err := srv.store.SubscribeToProjectSandboxUpdates(ctx, srv.handleProjectSandboxUpdate); err != nil {
					srv.logger.Error("Failed to subscribe to project-sandbox updates", zap.Error(err))
				}
			}()

			// Start the background updater that periodically refreshes the snapshot
			go srv.backgroundUpdater()

			// Start the dedicated XDS gRPC server
			go srv.StartServer()

			return nil
		},
		OnStop: func(ctx context.Context) error {
			// Gracefully stop the gRPC server
			srv.grpcServer.GracefulStop()
			return nil
		},
	})

	return srv
}

// backgroundUpdater periodically updates the XDS snapshot
func (s *Service) backgroundUpdater() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		<-ticker.C
		if err := s.UpdateSnapshot(); err != nil {
			s.logger.Error("Failed to update snapshot", zap.Error(err))
		}
	}
}

// AddOrUpdateRoute adds or updates a route for a project
func (s *Service) AddOrUpdateRoute(projectID inttypes.ProjectID, sandboxID inttypes.SandboxID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.routes[projectID] = sandboxID

	// Update the snapshot
	return s.UpdateSnapshot()
}

// RemoveRoute removes a route for a project
func (s *Service) RemoveRoute(projectID inttypes.ProjectID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.routes, projectID)

	// Update the snapshot
	return s.UpdateSnapshot()
}

func (s *Service) RegisterToGRPCServer(server *grpc.Server) {
	discovery.RegisterAggregatedDiscoveryServiceServer(server, s.server)
}

// UpdateSnapshot updates the XDS snapshot with the current routes
func (s *Service) UpdateSnapshot() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Create the resources
	var (
		clusters  []types.Resource
		endpoints []types.Resource
		routes    []types.Resource
		listeners []types.Resource
	)

	// Create the default route to the activator
	activatorCluster := &cluster.Cluster{
		Name:                 "activator_cluster",
		ConnectTimeout:       durationpb.New(1 * time.Second),
		ClusterDiscoveryType: &cluster.Cluster_Type{Type: cluster.Cluster_STRICT_DNS},
		LbPolicy:             cluster.Cluster_ROUND_ROBIN,
		LoadAssignment: &endpoint.ClusterLoadAssignment{
			ClusterName: "activator_cluster",
			Endpoints: []*endpoint.LocalityLbEndpoints{
				{
					LbEndpoints: []*endpoint.LbEndpoint{
						{
							HostIdentifier: &endpoint.LbEndpoint_Endpoint{
								Endpoint: &endpoint.Endpoint{
									Address: &core.Address{
										Address: &core.Address_SocketAddress{
											SocketAddress: &core.SocketAddress{
												Protocol: core.SocketAddress_TCP,
												Address:  "activator.sandbox.svc",
												PortSpecifier: &core.SocketAddress_PortValue{
													PortValue: 80,
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	clusters = append(clusters, activatorCluster)

	// Create a cluster and endpoint for each project
	for projectID, _ := range s.routes {
		clusterName := fmt.Sprintf("project_%s_cluster", projectID)
		hostname := s.k8sClient.GetProjectServiceHostname(projectID)

		// Create the cluster
		clusterResource := &cluster.Cluster{
			Name:                 clusterName,
			ConnectTimeout:       durationpb.New(1 * time.Second),
			ClusterDiscoveryType: &cluster.Cluster_Type{Type: cluster.Cluster_STRICT_DNS},
			LbPolicy:             cluster.Cluster_ROUND_ROBIN,
			LoadAssignment: &endpoint.ClusterLoadAssignment{
				ClusterName: clusterName,
				Endpoints: []*endpoint.LocalityLbEndpoints{
					{
						LbEndpoints: []*endpoint.LbEndpoint{
							{
								HostIdentifier: &endpoint.LbEndpoint_Endpoint{
									Endpoint: &endpoint.Endpoint{
										Address: &core.Address{
											Address: &core.Address_SocketAddress{
												SocketAddress: &core.SocketAddress{
													Protocol: core.SocketAddress_TCP,
													Address:  hostname,
													PortSpecifier: &core.SocketAddress_PortValue{
														PortValue: 8000,
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}
		clusters = append(clusters, clusterResource)
	}

	// Create the routes
	virtualRoutes := []*route.Route{}

	// Add routes for each project based on Project-Id header
	for projectID, _ := range s.routes {
		clusterName := fmt.Sprintf("project_%s_cluster", projectID)
		virtualRoute := &route.Route{
			Match: &route.RouteMatch{
				PathSpecifier: &route.RouteMatch_Prefix{
					Prefix: "/",
				},
				Headers: []*route.HeaderMatcher{
					{
						Name: "Project-Id",
						HeaderMatchSpecifier: &route.HeaderMatcher_StringMatch{
							StringMatch: &matcher.StringMatcher{
								MatchPattern: &matcher.StringMatcher_Exact{
									Exact: projectID.String(),
								},
							},
						},
					},
				},
			},
			Action: &route.Route_Route{
				Route: &route.RouteAction{
					ClusterSpecifier: &route.RouteAction_Cluster{
						Cluster: clusterName,
					},
				},
			},
		}
		virtualRoutes = append(virtualRoutes, virtualRoute)
	}

	// Add a fallback route to the activator (for all requests without a matching Project-Id header)
	defaultRoute := &route.Route{
		Match: &route.RouteMatch{
			PathSpecifier: &route.RouteMatch_Prefix{
				Prefix: "/",
			},
		},
		Action: &route.Route_Route{
			Route: &route.RouteAction{
				ClusterSpecifier: &route.RouteAction_Cluster{
					Cluster: "activator_cluster",
				},
			},
		},
	}
	virtualRoutes = append(virtualRoutes, defaultRoute)

	// Create the route configuration
	routeConfiguration := &route.RouteConfiguration{
		Name: "local_route",
		VirtualHosts: []*route.VirtualHost{
			{
				Name:    "local_service",
				Domains: []string{"*"},
				Routes:  virtualRoutes,
			},
		},
	}
	routes = append(routes, routeConfiguration)

	// Create the HTTP connection manager
	manager := &hcm.HttpConnectionManager{
		CodecType:  hcm.HttpConnectionManager_AUTO,
		StatPrefix: "ingress_http",
		RouteSpecifier: &hcm.HttpConnectionManager_RouteConfig{
			RouteConfig: routeConfiguration,
		},
		HttpFilters: []*hcm.HttpFilter{
			{
				Name: "envoy.filters.http.router",
				ConfigType: &hcm.HttpFilter_TypedConfig{
					TypedConfig: &anypb.Any{
						TypeUrl: "type.googleapis.com/envoy.extensions.filters.http.router.v3.Router",
					},
				},
			},
		},
	}

	// Convert the HTTP connection manager to an Any
	pbst, err := anypb.New(manager)
	if err != nil {
		return fmt.Errorf("failed to marshal HTTP connection manager: %w", err)
	}

	// Create the listener
	listenerResource := &listener.Listener{
		Name: "listener_0",
		Address: &core.Address{
			Address: &core.Address_SocketAddress{
				SocketAddress: &core.SocketAddress{
					Protocol: core.SocketAddress_TCP,
					Address:  "0.0.0.0",
					PortSpecifier: &core.SocketAddress_PortValue{
						PortValue: 8080,
					},
				},
			},
		},
		FilterChains: []*listener.FilterChain{
			{
				Filters: []*listener.Filter{
					{
						Name: "envoy.filters.network.http_connection_manager",
						ConfigType: &listener.Filter_TypedConfig{
							TypedConfig: pbst,
						},
					},
				},
			},
		},
	}
	listeners = append(listeners, listenerResource)

	// Log the routing configuration
	s.logger.Info("Updating XDS snapshot with Project-Id header-based routing",
		zap.Int("projects", len(s.routes)),
		zap.Bool("fallback_enabled", true))

	// Create the snapshot
	snapshot, err := cache.NewSnapshot(
		fmt.Sprintf("%d", time.Now().UnixNano()),
		map[resource.Type][]types.Resource{
			resource.ClusterType:  clusters,
			resource.EndpointType: endpoints,
			resource.RouteType:    routes,
			resource.ListenerType: listeners,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to create snapshot: %w", err)
	}

	// Set the snapshot for all nodes (we use the same for all)
	if err := s.snapshotCache.SetSnapshot(context.Background(), "envoy-node", snapshot); err != nil {
		return fmt.Errorf("failed to set snapshot: %w", err)
	}

	return nil
}

// loadInitialRoutes loads the initial routes from the store
func (s *Service) loadInitialRoutes(ctx context.Context) error {
	// Get all project-sandbox mappings from the store
	mappings, err := s.store.GetAllProjectSandboxMappings(ctx)
	if err != nil {
		return fmt.Errorf("failed to get project-sandbox mappings: %w", err)
	}

	s.logger.Info("Loaded initial project-sandbox mappings", zap.Int("count", len(mappings)))

	// Add routes for each mapping
	s.mu.Lock()
	for projectID, sandboxID := range mappings {
		s.routes[projectID] = sandboxID
	}
	s.mu.Unlock()

	// Generate the initial snapshot
	return s.UpdateSnapshot()
}

// handleProjectSandboxUpdate handles project-sandbox mapping updates from Redis pub/sub
func (s *Service) handleProjectSandboxUpdate(projectID inttypes.ProjectID, sandboxID inttypes.SandboxID, isRemoval bool) {
	// Log the update
	if isRemoval {
		s.logger.Info("Project sandbox mapping removed",
			zap.String("project_id", projectID.String()))

		// Remove the route
		if err := s.RemoveRoute(projectID); err != nil {
			s.logger.Error("Failed to remove route",
				zap.String("project_id", projectID.String()),
				zap.Error(err))
		}
	} else {
		s.logger.Info("Project sandbox mapping updated",
			zap.String("project_id", projectID.String()),
			zap.String("sandbox_id", sandboxID.String()))

		// Add or update the route
		if err := s.AddOrUpdateRoute(projectID, sandboxID); err != nil {
			s.logger.Error("Failed to add/update route",
				zap.String("project_id", projectID.String()),
				zap.String("sandbox_id", sandboxID.String()),
				zap.Error(err))
		}
	}
}

// StartServer starts the XDS gRPC server on a dedicated port
func (s *Service) StartServer() {
	// Use port 50051 for XDS gRPC server
	address := fmt.Sprintf("0.0.0.0:%d", s.config.XDSPort)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		s.logger.Fatal("Failed to listen for XDS gRPC server", zap.Error(err))
	}

	s.logger.Info("Starting XDS gRPC server", zap.String("address", address))
	if err := s.grpcServer.Serve(listener); err != nil {
		s.logger.Fatal("Failed to start XDS gRPC server", zap.Error(err))
	}
}
