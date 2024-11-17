package driver

import (
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/uthoplatforms/utho-go/utho"
	"k8s.io/mount-utils"
	"k8s.io/utils/exec"
)

const (
	DefaultDriverName = "csi.utho.com"
	defaultTimeout    = 1 * time.Minute
)

// UthoDriver struct
type UthoDriver struct {
	name     string
	endpoint string
	nodeID   string
	region   string
	client   utho.Client

	publishInfoVolumeName string
	mounter               *mount.SafeFormatAndMount
	resizer               *mount.ResizeFs

	isController bool
	waitTimeout  time.Duration

	log *logrus.Entry

	version string
}

func NewDriver(endpoint, token, driverName, version, region string, isDebug bool) (*UthoDriver, error) {
	if driverName == "" {
		driverName = DefaultDriverName
	}

	client, err := utho.NewClient(token)
	if err != nil {
		return nil, err
	}

	log := logrus.New().WithFields(logrus.Fields{
		"version": version,
	})

	var nodeId string

	if isDebug {
		nodeId = GenerateRandomString(10)
	} else {
		nodeId, err = GetNodeId(client)
		if err != nil {
			return nil, err
		}
	}
	fmt.Printf("node id %s:\n", nodeId)

	return &UthoDriver{
		name:                  driverName,
		publishInfoVolumeName: driverName + "/volume-name",

		endpoint: endpoint,
		nodeID:   nodeId,
		region:   region,
		client:   client,

		log: log,
		mounter: &mount.SafeFormatAndMount{
			Interface: mount.New(""),
			Exec:      exec.New(),
		},

		resizer: mount.NewResizeFs(mount.SafeFormatAndMount{
			Interface: mount.New(""),
			Exec:      exec.New(),
		}.Exec),

		version: version,
	}, nil
}

func (d *UthoDriver) Run() {
	server := NewNonBlockingGRPCServer()
	identity := NewUthoIdentityServer(d)
	controller := NewUthoControllerServer(d)
	node := NewUthoNodeDriver(d)

	server.Start(d.endpoint, identity, controller, node)
	server.Wait()
}
