package main

import (
	"context"
	"fmt"
	"os"

	"github.com/libdns/ionos"
)

func main() {
	token := os.Getenv("LIBDNS_IONOS_TOKEN")
	if token == "" {
		panic("LIBDNS_IONOS_TOKEN not set")
	}

	zone := os.Getenv("LIBDNS_IONOS_ZONE")
	if zone == "" {
		panic("LIBDNS_IONOS_ZONE not set")
	}

	p := &ionos.Provider{
		AuthAPIToken: token,
	}

	zones, err := p.ListZones(context.TODO())
	if err != nil {
		panic(err)
	}
	fmt.Print("Zones:\n")
	for _, z := range zones {
		fmt.Printf("%+v\n", z)
	}

	fmt.Printf("\nRecord in Zone %s\n", zone)
	records, err := p.GetRecords(context.TODO(), zone)
	if err != nil {
		panic(err)
	}

	for _, r := range records {
		fmt.Printf("%+v\n", r.RR())
	}
}