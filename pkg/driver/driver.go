package driver

import (
	"time"

	"github.com/sirupsen/logrus"
	"github.com/uthoplatforms/utho-go/utho"
	"k8s.io/mount-utils"
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

	publishVolumeID string
	mountID         string
	mounter         *mount.SafeFormatAndMount
	resizer         *mount.ResizeFs

	isController bool
	waitTimeout  time.Duration

	log *logrus.Entry

	version string
}

func NewDriver(endpoint, token, driverName, version, region string) (*UthoDriver, error) {
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

	return &UthoDriver{
		name:     driverName,
		endpoint: endpoint,
		region:   region,
		client:   client,

		log: log,

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
