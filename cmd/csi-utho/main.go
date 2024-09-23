package main

import (
	"flag"
	"log"

	"github.com/uthoplatforms/csi-utho/pkg/driver"
)

var version string

func main() {
	var (
		endpoint   = flag.String("endpoint", "unix:///var/lib/kubelet/plugins/"+driver.DefaultDriverName+"/csi.sock", "CSI endpoint")
		token      = flag.String("token", "", "Utho API Token")
		region     = flag.String("region", "", "Utho region slug.")
		driverName = flag.String("driver-name", driver.DefaultDriverName, "Name of driver")
	)
	flag.Parse()

	if version == "" {
		log.Fatal("version must be defined at compilation")
	}

	d, err := driver.NewDriver(*endpoint, *token, *driverName, version, *region)
	if err != nil {
		log.Fatalln(err)
	}

	d.Run()
}
