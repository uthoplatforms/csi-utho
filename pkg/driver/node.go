package driver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	diskPath   = "/dev/disk/by-id"
	diskPrefix = "virtio-"

	mkDirMode = 0750

	volumeModeFilesystem = "filesystem"
)

var _ csi.NodeServer = &UthoNodeServer{}

// UthoNodeServer type provides the UthoDriver
type UthoNodeServer struct {
	csi.UnimplementedNodeServer
	Driver *UthoDriver
}

// NewUthoNodeDriver provides a UthoNodeServer
func NewUthoNodeDriver(driver *UthoDriver) *UthoNodeServer {
	return &UthoNodeServer{Driver: driver}
}

// NodeStageVolume mounts the volume to a staging path on the node.
func (n *UthoNodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "NodeStageVolume Volume ID must be provided")
	}

	if req.StagingTargetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "NodeStageVolume Staging Target Path must be provided")
	}

	if req.VolumeCapability == nil {
		return nil, status.Error(codes.InvalidArgument, "NodeStageVolume Volume Capability must be provided")
	}

	n.Driver.log.WithFields(logrus.Fields{
		"volume":   req.VolumeId,
		"target":   req.StagingTargetPath,
		"capacity": req.VolumeCapability,
	}).Info("Node Stage Volume: called")

	volumeID, ok := req.GetPublishContext()[n.Driver.mountID]
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "Could not find the volume id")
	}

	source := getDeviceByPath(volumeID)
	target := req.StagingTargetPath
	mountBlk := req.VolumeCapability.GetMount()
	options := mountBlk.MountFlags

	fsType := "ext4"
	if mountBlk.FsType != "" {
		fsType = mountBlk.FsType
	}

	n.Driver.log.WithFields(logrus.Fields{
		"volume":   req.VolumeId,
		"target":   req.StagingTargetPath,
		"capacity": req.VolumeCapability,
	}).Infof("Node Stage Volume: creating directory target %s\n", target)

	err := os.MkdirAll(target, mkDirMode)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	n.Driver.log.WithFields(logrus.Fields{
		"volume":   req.VolumeId,
		"target":   req.StagingTargetPath,
		"capacity": req.VolumeCapability,
	}).Infof("Node Stage Volume: directory created for target %s\n", target)

	n.Driver.log.WithFields(logrus.Fields{
		"volume":   req.VolumeId,
		"target":   req.StagingTargetPath,
		"capacity": req.VolumeCapability,
	}).Info("Node Stage Volume: attempting format and mount")

	if err := n.Driver.mounter.FormatAndMount(source, target, fsType, options); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	if _, err := os.Stat(source); err == nil {
		needResize, err := n.Driver.resizer.NeedResize(source, target)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "could not determine if volume %q needs to be resized: %v", req.VolumeId, err)
		}

		if needResize {
			n.Driver.log.WithFields(logrus.Fields{
				"volume":   req.VolumeId,
				"target":   req.StagingTargetPath,
				"capacity": req.VolumeCapability,
			}).Info("Node Stage Volume: resizing volume")

			if _, err := n.Driver.resizer.Resize(source, target); err != nil {
				return nil, status.Errorf(codes.Internal, "could not resize volume %q:  %v", req.VolumeId, err)
			}
		}
	}
	n.Driver.log.Info("Node Stage Volume: volume staged")
	return &csi.NodeStageVolumeResponse{}, nil
}
