package driver

import (
	"context"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
)

var _ csi.IdentityServer = &UthoIdentityServer{}

// UthoIdentityServer provides the Driver
type UthoIdentityServer struct {
	csi.UnimplementedIdentityServer
	Driver *UthoDriver
}

// NewUthoIdentityServer initializes the UthoIdentityServer
func NewUthoIdentityServer(driver *UthoDriver) *UthoIdentityServer {
	return &UthoIdentityServer{Driver: driver}
}

func (uthoIdentity *UthoIdentityServer) GetPluginInfo(ctx context.Context, req *csi.GetPluginInfoRequest) (*csi.GetPluginInfoResponse, error) {
	uthoIdentity.Driver.log.Info("UthoIdentityServer.GetPluginInfo called")

	return &csi.GetPluginInfoResponse{
		Name:          uthoIdentity.Driver.name,
		VendorVersion: uthoIdentity.Driver.version,
	}, nil
}

func (uthoIdentity *UthoIdentityServer) GetPluginCapabilities(ctx context.Context, req *csi.GetPluginCapabilitiesRequest) (*csi.GetPluginCapabilitiesResponse, error) {
	uthoIdentity.Driver.log.Infof("UthoIdentityServer.GetPluginCapabilities called with request : %v", req)

	return &csi.GetPluginCapabilitiesResponse{
		Capabilities: []*csi.PluginCapability{
			{
				Type: &csi.PluginCapability_Service_{
					Service: &csi.PluginCapability_Service{
						Type: csi.PluginCapability_Service_CONTROLLER_SERVICE,
					},
				},
			},
		},
	}, nil
}

// Probe returns the health and readiness of the plugin
func (uthoIdentity *UthoIdentityServer) Probe(ctx context.Context, req *csi.ProbeRequest) (*csi.ProbeResponse, error) {
	uthoIdentity.Driver.log.Infof("UthoIdentityServer.Probe called with request : %v", req)

	return &csi.ProbeResponse{}, nil
}
