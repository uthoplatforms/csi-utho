package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/uthoplatforms/csi-utho/pkg/driver"
)

func main() {
	var version string
	var (
		endpoint   = flag.String("endpoint", "unix:///var/lib/kubelet/plugins/"+driver.DefaultDriverName+"/csi.sock", "CSI endpoint")
		token      = flag.String("token", "", "Utho API Token")
		dcslug     = flag.String("dcslug", "inmumbaizone2", "Utho dcslug.")
		driverName = flag.String("driver-name", driver.DefaultDriverName, "Name of driver")
		debug      = flag.Bool("debug", false, "Is debug")
	)
	st := ""
	pt := &st
	fmt.Print(pt)
	flag.Parse()
	version = "1.0.0"
	if version == "" {
		log.Fatal("version must be defined at compilation")
	}

	d, err := driver.NewDriver(*endpoint, *token, *driverName, version, *dcslug, *debug)
	if err != nil {
		log.Fatalln(err)
	}

	d.Run()
}
