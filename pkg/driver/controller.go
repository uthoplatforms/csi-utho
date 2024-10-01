package driver

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/sirupsen/logrus"
	"github.com/uthoplatforms/utho-go/utho"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ControllerServer struct {
	csi.UnimplementedControllerServer
}

const (
	_   = iota
	kiB = 1 << (10 * iota)
	miB
	giB
	tiB
)

const (
	// minimumVolumeSizeInBytes is used to validate that the user is not trying
	// to create a volume that is smaller than what we support
	minimumVolumeSizeInBytes int64 = 1 * giB

	// maximumVolumeSizeInBytes is used to validate that the user is not trying
	// to create a volume that is larger than what we support
	maximumVolumeSizeInBytes int64 = 16 * tiB

	// defaultVolumeSizeInBytes is used when the user did not provide a size or
	// the size they provided did not satisfy our requirements
	defaultVolumeSizeInBytes int64 = 16 * giB
)

var (
	supportedVolCapabilities = &csi.VolumeCapability_AccessMode{
		Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
	}
)

var _ csi.ControllerServer = &UthoControllerServer{}

// UthoControllerServer is the struct type for the UthoDriver
type UthoControllerServer struct {
	csi.UnimplementedControllerServer
	Driver *UthoDriver
}

// NewUthoControllerServer returns a UthoControllerServer
func NewUthoControllerServer(driver *UthoDriver) *UthoControllerServer {
	return &UthoControllerServer{Driver: driver}
}

// CreateVolume provisions a new volume on behalf of the user
func (c *UthoControllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	volName := req.Name
	if volName == "" {
		return nil, status.Error(codes.InvalidArgument, "CreateVolume Name is missing")
	}
	if len(req.VolumeCapabilities) == 0 {
		return nil, status.Error(codes.InvalidArgument, "CreateVolume Volume Capabilities is missing")
	}
	if req.Parameters["dcslug"] == "" {
		return nil, status.Error(codes.InvalidArgument, "CreateVolume Volume parameter `dcslug` is missing")
	}
	if req.Parameters["iops"] == "" {
		return nil, status.Error(codes.InvalidArgument, "CreateVolume Volume parameter `iops` is missing")
	}
	if req.Parameters["throughput"] == "" {
		return nil, status.Error(codes.InvalidArgument, "CreateVolume Volume parameter `throughput` is missing")
	}

	// Validate
	if !isValidCapability(req.VolumeCapabilities) {
		return nil, status.Errorf(codes.InvalidArgument, "CreateVolume Volume capability is not compatible: %v", req)
	}

	size, err := extractStorage(req.CapacityRange)
	if err != nil {
		return nil, status.Errorf(codes.OutOfRange, "invalid capacity range: %v", err)
	}

	c.Driver.log.WithFields(logrus.Fields{
		"volume-name":  volName,
		"size":         size,
		"capabilities": req.VolumeCapabilities,
	}).Info("Create Volume: called")

	// check that the volume doesnt already exist
	volumes, err := c.Driver.client.Ebs().List()
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	for _, volume := range volumes {
		if volume.Name == volName {
			byteSize, err := strconv.ParseFloat(volume.Size, 64)
			if err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}

			// return erro if volume exist and request with diffrent size
			x := int64(byteSize) * giB
			x1 := size
			if x != x1 {
				return nil, status.Error(codes.AlreadyExists, "Volume with the same name but different volume already exists")
			}

			return &csi.CreateVolumeResponse{
				Volume: &csi.Volume{
					VolumeId:      volume.ID,
					CapacityBytes: int64(byteSize) * giB,
				},
			}, nil
		}
	}

	// if applicable, create volume
	params := utho.CreateEBSParams{
		Name:       volName,
		Dcslug:     req.Parameters["dcslug"],
		Disk:       strconv.Itoa(bytesToGB(size)),
		Iops:       req.Parameters["iops"],
		Throughput: req.Parameters["throughput"],
		DiskType:   "SSD",
	}
	ebsCreateRes, err := c.Driver.client.Ebs().Create(params)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	res := &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      ebsCreateRes.ID,
			CapacityBytes: size,
		},
	}

	c.Driver.log.WithFields(logrus.Fields{
		"size":        size,
		"volume-id":   ebsCreateRes.ID,
		"volume-name": volName,
		"volume-size": size,
	}).Info("Create Volume: created volume")

	return res, nil
}

// DeleteVolume performs the volume deletion
func (c *UthoControllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "DeleteVolume VolumeID is missing")
	}

	c.Driver.log.WithFields(logrus.Fields{
		"volume-id": req.VolumeId,
	}).Info("Delete volume: called")

	volumes, err := c.Driver.client.Ebs().List()
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	// chechk if exist
	exists := false
	for _, volume := range volumes {
		if volume.ID == req.VolumeId {
			exists = true
			break
		}
	}
	if !exists {
		return &csi.DeleteVolumeResponse{}, nil
	}

	_, err = c.Driver.client.Ebs().Delete(req.VolumeId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "cannot delete volume, %v", err.Error())
	}

	c.Driver.log.WithFields(logrus.Fields{
		"volume-id": req.VolumeId,
	}).Info("Delete Volume: deleted")

	return &csi.DeleteVolumeResponse{}, nil
}

// ControllerPublishVolume performs the volume publish for the controller
func (c *UthoControllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) { //nolint:lll,gocyclo
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "ControllerPublishVolume Volume ID is missing")
	}

	if req.NodeId == "" {
		return nil, status.Error(codes.InvalidArgument, "ControllerPublishVolume Node ID is missing")
	}

	if req.VolumeCapability == nil {
		return nil, status.Error(codes.InvalidArgument, "ControllerPublishVolume VolumeCapability is missing")
	}

	if req.Readonly {
		return nil, status.Error(codes.InvalidArgument, "ControllerPublishVolume read only is not currently supported")
	}

	volume, err := c.Driver.client.Ebs().Read(req.VolumeId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "cannot get volume: %v", err.Error())
	}

	if _, err = c.Driver.client.CloudInstances().Read(req.NodeId); err != nil {
		return nil, status.Errorf(codes.NotFound, "cannot get node: %v", err.Error())
	}

	// node is already attached, do nothing
	if volume.Cloudid == req.NodeId {
		return &csi.ControllerPublishVolumeResponse{
			PublishContext: map[string]string{
				c.Driver.publishInfoVolumeName: volume.Name,
			},
		}, nil
	}

	// assuming its attached & to the wrong node
	if volume.Cloudid != "" {
		return nil, status.Errorf(codes.FailedPrecondition,
			"cannot attach volume to node because it is already attached to a different node ID: %v node name: %v", volume.Cloudid, volume.Name)
	}

	c.Driver.log.WithFields(logrus.Fields{
		"volume-id": req.VolumeId,
		"node-id":   req.NodeId,
	}).Info("Controller Publish Volume: called")

	params := utho.AttachEBSParams{
		EBSId:      req.VolumeId,
		ResourceId: req.NodeId,
		Type:       "cloud",
	}
	_, err = c.Driver.client.Ebs().Attach(params)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "cannot attach volume, %v", err.Error())
	}

	// attachReady := false
	// for i := 0; i < volumeStatusCheckRetries; i++ {
	// 	time.Sleep(volumeStatusCheckInterval * time.Second)
	// 	bs, _, err := c.Driver.client.BlockStorage.Get(ctx, volume.ID) //nolint:bodyclose
	// 	if err != nil {
	// 		return nil, status.Error(codes.Internal, err.Error())
	// 	}

	// 	if bs.AttachedToInstance == req.NodeId {
	// 		attachReady = true
	// 		break
	// 	}
	// }

	// if !attachReady {
	// 	return nil, status.Errorf(codes.Internal, "volume is not attached to node after %v seconds", volumeStatusCheckRetries)
	// }

	c.Driver.log.WithFields(logrus.Fields{
		"volume-id": req.VolumeId,
		"node-id":   req.NodeId,
	}).Info("Controller Publish Volume: published")

	return &csi.ControllerPublishVolumeResponse{
		PublishContext: map[string]string{
			c.Driver.publishInfoVolumeName: volume.Name,
		},
	}, nil
}

// ControllerUnpublishVolume performs the volume un-publish
func (c *UthoControllerServer) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) { //nolint:lll
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "ControllerUnpublishVolume Volume ID is missing")
	}

	if req.NodeId == "" {
		return nil, status.Error(codes.InvalidArgument, "ControllerUnpublishVolume Node ID is missing")
	}

	c.Driver.log.WithFields(logrus.Fields{
		"volume-id": req.VolumeId,
		"node-id":   req.NodeId,
	}).Info("Controller Publish Unpublish: called")

	volume, err := c.Driver.client.Ebs().Read(req.VolumeId)
	if err != nil {
		return &csi.ControllerUnpublishVolumeResponse{}, nil
	}

	// node is already unattached, do nothing
	if volume.Cloudid == "" {
		return &csi.ControllerUnpublishVolumeResponse{}, nil
	}

	if _, err = c.Driver.client.CloudInstances().Read(req.NodeId); err != nil {
		return nil, status.Errorf(codes.NotFound, "cannot get node: %v", err.Error())
	}

	params := utho.AttachEBSParams{
		EBSId:      req.VolumeId,
		ResourceId: req.NodeId,
		Type:       "cloud",
	}

	_, err = c.Driver.client.Ebs().Dettach(params)
	if err != nil {
		if strings.Contains(err.Error(), "Block storage volume is not currently attached to a server") {
			return &csi.ControllerUnpublishVolumeResponse{}, nil
		}
		return nil, status.Errorf(codes.Internal, "cannot detach volume: %v", err.Error())
	}

	c.Driver.log.WithFields(logrus.Fields{
		"volume-id": req.VolumeId,
		"node-id":   req.NodeId,
	}).Info("Controller Unublish Volume: unpublished")

	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

// ValidateVolumeCapabilities checks if requested capabilities are supported
func (c *UthoControllerServer) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) { //nolint:lll
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "ValidateVolumeCapabilities Volume ID is missing")
	}

	if req.VolumeCapabilities == nil {
		return nil, status.Error(codes.InvalidArgument, "ValidateVolumeCapabilities Volume Capabilities is missing")
	}

	if _, err := c.Driver.client.Ebs().Read(req.VolumeId); err != nil {
		return nil, status.Errorf(codes.NotFound, "cannot get volume: %v", err.Error())
	}

	res := &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
			VolumeCapabilities: []*csi.VolumeCapability{
				{
					AccessMode: supportedVolCapabilities,
				},
			},
		},
	}

	return res, nil
}

// ListVolumes performs the list volume function
func (c *UthoControllerServer) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	if req.StartingToken != "" {
		_, err := strconv.Atoi(req.StartingToken)
		if err != nil {
			return nil, status.Errorf(codes.Aborted, "ListVolumes starting_token is invalid: %s", err)
		}
	}

	var entries []*csi.ListVolumesResponse_Entry
	volumes, err := c.Driver.client.Ebs().List()
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	for _, volume := range volumes {
		byteSize, err := strconv.ParseFloat(volume.Size, 64)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		entries = append(entries, &csi.ListVolumesResponse_Entry{
			Volume: &csi.Volume{
				VolumeId:      volume.ID,
				CapacityBytes: int64(byteSize),
			},
		})
	}

	res := &csi.ListVolumesResponse{
		Entries: entries,
	}

	c.Driver.log.WithFields(logrus.Fields{
		"volumes": entries,
	}).Info("List Volumes")

	return res, nil
}

// ControllerGetCapabilities get capabilities of the controller
func (c *UthoControllerServer) ControllerGetCapabilities(context.Context, *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) { //nolint:lll
	capability := func(capability csi.ControllerServiceCapability_RPC_Type) *csi.ControllerServiceCapability {
		return &csi.ControllerServiceCapability{
			Type: &csi.ControllerServiceCapability_Rpc{
				Rpc: &csi.ControllerServiceCapability_RPC{
					Type: capability,
				},
			},
		}
	}

	var capabilities []*csi.ControllerServiceCapability
	for _, caps := range []csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
	} {
		capabilities = append(capabilities, capability(caps))
	}

	resp := &csi.ControllerGetCapabilitiesResponse{
		Capabilities: capabilities,
	}

	c.Driver.log.WithFields(logrus.Fields{
		"response": resp,
		"method":   "controller-get-capabilities",
	})

	return resp, nil
}

func isValidCapability(caps []*csi.VolumeCapability) bool {
	for _, capacity := range caps {
		if capacity == nil {
			return false
		}

		accessMode := capacity.GetAccessMode()
		if accessMode == nil {
			return false
		}

		if accessMode.GetMode() != supportedVolCapabilities.GetMode() {
			return false
		}

		accessType := capacity.GetAccessType()
		switch accessType.(type) {
		case *csi.VolumeCapability_Block:
		case *csi.VolumeCapability_Mount:
		default:
			return false
		}
	}
	return true
}

func formatBytes(inputBytes int64) string {
	output := float64(inputBytes)
	unit := ""

	switch {
	case inputBytes >= tiB:
		output = output / tiB
		unit = "Ti"
	case inputBytes >= giB:
		output = output / giB
		unit = "Gi"
	case inputBytes >= miB:
		output = output / miB
		unit = "Mi"
	case inputBytes >= kiB:
		output = output / kiB
		unit = "Ki"
	case inputBytes == 0:
		return "0"
	}

	result := strconv.FormatFloat(output, 'f', 1, 64)
	result = strings.TrimSuffix(result, ".0")
	return result + unit
}

// extractStorage extracts the storage size in bytes from the given capacity
// range. If the capacity range is not satisfied it returns the default volume
// size. If the capacity range is above supported sizes, it returns an
// error. If the capacity range is below supported size, it returns the minimum supported size
func extractStorage(capRange *csi.CapacityRange) (int64, error) {
	if capRange == nil {
		return defaultVolumeSizeInBytes, nil
	}

	requiredBytes := capRange.GetRequiredBytes()
	requiredSet := 0 < requiredBytes
	limitBytes := capRange.GetLimitBytes()
	limitSet := 0 < limitBytes

	if !requiredSet && !limitSet {
		return defaultVolumeSizeInBytes, nil
	}

	if requiredSet && limitSet && limitBytes < requiredBytes {
		return 0, fmt.Errorf("limit (%v) can not be less than required (%v) size", formatBytes(limitBytes), formatBytes(requiredBytes))
	}

	if requiredSet && !limitSet && requiredBytes < minimumVolumeSizeInBytes {
		return minimumVolumeSizeInBytes, fmt.Errorf("limit (%v) can not be less than minimum supported volume size (%v)", formatBytes(limitBytes), formatBytes(minimumVolumeSizeInBytes))
	}

	if limitSet && limitBytes < minimumVolumeSizeInBytes {
		return 0, fmt.Errorf("limit (%v) can not be less than minimum supported volume size (%v)", formatBytes(limitBytes), formatBytes(minimumVolumeSizeInBytes))
	}

	if requiredSet && requiredBytes > maximumVolumeSizeInBytes {
		return 0, fmt.Errorf("required (%v) can not exceed maximum supported volume size (%v)", formatBytes(requiredBytes), formatBytes(maximumVolumeSizeInBytes))
	}

	if !requiredSet && limitSet && limitBytes > maximumVolumeSizeInBytes {
		return 0, fmt.Errorf("limit (%v) can not exceed maximum supported volume size (%v)", formatBytes(limitBytes), formatBytes(maximumVolumeSizeInBytes))
	}

	if requiredSet && limitSet && requiredBytes == limitBytes {
		return requiredBytes, nil
	}

	if requiredSet {
		return requiredBytes, nil
	}

	if limitSet {
		return limitBytes, nil
	}

	return defaultVolumeSizeInBytes, nil
}

func bytesToGB(bytes int64) int {
	const bytesInGB = 1024 * 1024 * 1024

	return int(bytes / bytesInGB)
}
