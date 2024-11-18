package driver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
	"k8s.io/mount-utils"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	diskPath   = "/dev/disk/by-id"
	diskPrefix = "virtio-uthostorage-"

	mkDirMode = 0750

	maxVolumesPerNode = 11

	volumeModeBlock      = "block"
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

// NodeStageVolume provides stages the node volume
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

	volumeID, ok := req.GetPublishContext()[n.Driver.publishVolumeID]
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

// NodeUnstageVolume provides the node volume unstage functionality
func (n *UthoNodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) { //nolint:dupl,lll
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "VolumeID must be provided")
	}

	if req.StagingTargetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "Staging Target Path must be provided")
	}

	n.Driver.log.WithFields(logrus.Fields{
		"volume-id":           req.VolumeId,
		"staging-target-path": req.StagingTargetPath,
	}).Info("Node Unstage Volume: called")

	n.Driver.log.WithFields(logrus.Fields{
		"volume-id":   req.VolumeId,
		"target-path": req.StagingTargetPath,
	}).Info("Node Unpublish Volume: called")

	mounted, err := n.isMounted(req.StagingTargetPath)
	if err != nil {
		return nil, err
	}

	if mounted {
		n.Driver.log.Info("unmounting the staging target path")

		err := n.Driver.mounter.Unmount(req.StagingTargetPath)
		if err != nil {
			return nil, err
		}
	} else {
		n.Driver.log.Info("staging target path is already unmounted")
	}

	n.Driver.log.Info("Node Unstage Volume: volume unstaged")
	return &csi.NodeUnstageVolumeResponse{}, nil
}

// NodePublishVolume allows the volume publish
func (n *UthoNodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) { //nolint:lll
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "VolumeID must be provided")
	}

	if req.StagingTargetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "Staging Target Path must be provided")
	}

	if req.TargetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "Target Path must be provided")
	}

	log := n.Driver.log.WithFields(logrus.Fields{
		"volume_id":           req.VolumeId,
		"staging_target_path": req.StagingTargetPath,
		"target_path":         req.TargetPath,
	})
	log.Info("Node Publish Volume: called")

	options := []string{"bind"}
	if req.Readonly {
		options = append(options, "ro")
	}

	mnt := req.VolumeCapability.GetMount()
	options = append(options, mnt.MountFlags...)

	fsType := "ext4"
	if mnt.FsType != "" {
		fsType = mnt.FsType
	}

	err := os.MkdirAll(req.TargetPath, mkDirMode)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	err = n.Driver.mounter.Mount(req.StagingTargetPath, req.TargetPath, fsType, options)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	n.Driver.log.Info("Node Publish Volume: published")
	return &csi.NodePublishVolumeResponse{}, nil
}

// NodeUnpublishVolume allows the volume to be unpublished
func (n *UthoNodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) { //nolint:dupl,lll
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "VolumeID must be provided")
	}

	if req.TargetPath == "" {
		return nil, status.Error(codes.InvalidArgument, "Target Path must be provided")
	}

	n.Driver.log.WithFields(logrus.Fields{
		"volume-id":   req.VolumeId,
		"target-path": req.TargetPath,
	}).Info("Node Unpublish Volume: called")

	mounted, err := n.isMounted(req.TargetPath)
	if err != nil {
		return nil, err
	}

	if mounted {
		n.Driver.log.Info("unmounting the staging target path")

		err := n.Driver.mounter.Unmount(req.TargetPath)
		if err != nil {
			return nil, err
		}
	} else {
		n.Driver.log.Info("staging target path is already unmounted")
	}

	n.Driver.log.Info("Node Publish Volume: unpublished")
	return &csi.NodeUnpublishVolumeResponse{}, nil
}

// NodeGetVolumeStats provides the volume stats
func (n *UthoNodeServer) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) { //nolint:lll
	if req.VolumeId == "" {
		return nil, status.Error(codes.InvalidArgument, "NodeGetVolumeStats Volume ID must be provided")
	}

	volumePath := req.VolumePath
	if volumePath == "" {
		return nil, status.Error(codes.InvalidArgument, "NodeGetVolumeStats Volume Path must be provided")
	}

	log := n.Driver.log.WithFields(logrus.Fields{
		"volume_id":   req.VolumeId,
		"volume_path": req.VolumePath,
		"method":      "node_get_volume_stats",
	})
	log.Info("node get volume stats called")

	mounted, err := n.isMounted(volumePath)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to check if volume path %q is mounted: %s", volumePath, err)
	}

	if !mounted {
		return nil, status.Errorf(codes.NotFound, "volume path %q is not mounted", volumePath)
	}

	statfs := &unix.Statfs_t{}
	err = unix.Statfs(volumePath, statfs)
	if err != nil {
		return nil, err
	}

	availableBytes := int64(statfs.Bavail) * int64(statfs.Bsize)                    //nolint:unconvert // 32bit builds fail otherwise
	usedBytes := (int64(statfs.Blocks) - int64(statfs.Bfree)) * int64(statfs.Bsize) //nolint:unconvert // 32bit builds fail otherwise
	totalBytes := int64(statfs.Blocks) * int64(statfs.Bsize)                        //nolint:unconvert // 32bit builds fail otherwise
	totalInodes := int64(statfs.Files)
	availableInodes := int64(statfs.Ffree)
	usedInodes := totalInodes - availableInodes

	log.WithFields(logrus.Fields{
		"volume_mode":      volumeModeFilesystem,
		"bytes_available":  availableBytes,
		"bytes_total":      totalBytes,
		"bytes_used":       usedBytes,
		"inodes_available": availableInodes,
		"inodes_total":     totalInodes,
		"inodes_used":      usedInodes,
	}).Info("node capacity statistics retrieved")

	return &csi.NodeGetVolumeStatsResponse{
		Usage: []*csi.VolumeUsage{
			{
				Available: availableBytes,
				Total:     totalBytes,
				Used:      usedBytes,
				Unit:      csi.VolumeUsage_BYTES,
			},
			{
				Available: availableInodes,
				Total:     totalInodes,
				Used:      usedInodes,
				Unit:      csi.VolumeUsage_INODES,
			},
		},
	}, nil
}

// NodeExpandVolume provides the node volume expansion
func (n *UthoNodeServer) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	log := n.Driver.log.WithFields(logrus.Fields{
		"volume_id":   req.VolumeId,
		"volume_path": req.VolumePath,
		"method":      "NodeExpandVolume",
	})

	n.Driver.log.WithFields(logrus.Fields{
		"required_bytes": req.CapacityRange.RequiredBytes,
	}).Info("Node Expand Volume: called")

	devicePath, _, err := mount.GetDeviceNameFromMount(mount.New(""), req.VolumePath)
	if err != nil {
		log.Infof("failed to determine mount path for %s: %s", req.VolumePath, err)
		return nil, fmt.Errorf("failed to determine mount path for %s: %s", req.VolumePath, err)
	}

	log.Infof("attempting to resize devicepath: %s", devicePath)

	if _, err := n.Driver.resizer.Resize(devicePath, req.VolumePath); err != nil {
		log.Infof("failed to resize volume: %s", err)
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to resize volume: %s", err))
	}

	return &csi.NodeExpandVolumeResponse{
		CapacityBytes: req.CapacityRange.RequiredBytes,
	}, nil
}

// NodeGetCapabilities provides the node capabilities
func (n *UthoNodeServer) NodeGetCapabilities(context.Context, *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	nodeCapabilities := []*csi.NodeServiceCapability{
		{
			Type: &csi.NodeServiceCapability_Rpc{
				Rpc: &csi.NodeServiceCapability_RPC{
					Type: csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
				},
			},
		},
		{
			Type: &csi.NodeServiceCapability_Rpc{
				Rpc: &csi.NodeServiceCapability_RPC{
					Type: csi.NodeServiceCapability_RPC_GET_VOLUME_STATS,
				},
			},
		},
	}

	n.Driver.log.WithFields(logrus.Fields{
		"capabilities": nodeCapabilities,
	}).Info("Node Get Capabilities: called")

	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: nodeCapabilities,
	}, nil
}

// NodeGetInfo provides the node info
func (n *UthoNodeServer) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	n.Driver.log.WithFields(logrus.Fields{}).Info("Node Get Info: called")

	x := csi.NodeGetInfoResponse{
		NodeId:            n.Driver.nodeID,
		MaxVolumesPerNode: maxVolumesPerNode,
		AccessibleTopology: &csi.Topology{
			Segments: map[string]string{
				"region": n.Driver.region,
			},
		},
	}
	return &x, nil
}

func getDeviceByPath(volumeID string) string {
	return filepath.Join(diskPath, fmt.Sprintf("%s%s", diskPrefix, volumeID))
}

type findmntResponse struct {
	FileSystems []fileSystem `json:"filesystems"`
}
type fileSystem struct {
	Target      string `json:"target"`
	Propagation string `json:"propagation"`
	FsType      string `json:"fstype"`
	Options     string `json:"options"`
}

func (n *UthoNodeServer) isMounted(target string) (bool, error) {
	if target == "" {
		return false, errors.New("target is not specified for checking the mount")
	}

	findmntCmd := "findmnt"
	_, err := exec.LookPath(findmntCmd)
	if err != nil {
		if err == exec.ErrNotFound {
			return false, fmt.Errorf("%q executable not found in $PATH", findmntCmd)
		}
		return false, err
	}

	findmntArgs := []string{"-o", "TARGET,PROPAGATION,FSTYPE,OPTIONS", "-M", target, "-J"}

	n.Driver.log.WithFields(logrus.Fields{
		"cmd":  findmntCmd,
		"args": findmntArgs,
	}).Info("checking if target is mounted")

	out, err := exec.Command(findmntCmd, findmntArgs...).CombinedOutput()
	if err != nil {
		// findmnt exits with non zero exit status if it couldn't find anything
		if strings.TrimSpace(string(out)) == "" {
			return false, nil
		}

		return false, fmt.Errorf("checking mounted failed: %v cmd: %q output: %q",
			err, findmntCmd, string(out))
	}

	// no response means there is no mount
	if string(out) == "" {
		return false, nil
	}

	var resp *findmntResponse
	err = json.Unmarshal(out, &resp)
	if err != nil {
		return false, fmt.Errorf("couldn't unmarshal data: %q: %s", string(out), err)
	}

	targetFound := false
	for _, fs := range resp.FileSystems {
		// check if the mount is propagated correctly. It should be set to shared.
		if fs.Propagation != "shared" {
			return true, fmt.Errorf("mount propagation for target %q is not enabled", target)
		}

		// the mountpoint should match as well
		if fs.Target == target {
			targetFound = true
		}
	}

	return targetFound, nil
}
