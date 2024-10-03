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
		region     = flag.String("region", "inmumbaizone2", "Utho region slug.")
		driverName = flag.String("driver-name", driver.DefaultDriverName, "Name of driver")
		debug      = flag.Bool("debug", false, "Is debug")
	)

	flag.Parse()
	if version == "" {
		log.Fatal("version must be defined at compilation")
	}

	d, err := driver.NewDriver(*endpoint, *token, *driverName, version, *region, *debug)
	if err != nil {
		log.Fatalln(err)
	}

	d.Run()
}
