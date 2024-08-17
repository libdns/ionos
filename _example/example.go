package main

import (
	"context"
	"encoding/json"
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

	records, err := p.GetRecords(context.TODO(), zone)
	if err != nil {
		panic(err)
	}

	out, _ := json.MarshalIndent(records, "", "  ")
	fmt.Println(string(out))
}
