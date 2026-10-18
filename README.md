# IONOS DNS API for `libdns`

This package implements the libdns interfaces for the [IONOS DNS
API](https://developer.hosting.ionos.de/docs/dns)

## Authenticating

To authenticate you need to supply a IONOS API Key, as described on
https://developer.hosting.ionos.de/docs/getstarted

## Restricting writes to zones

IONOS API keys grant access to all zones of an account. Set `AllowedZones`
(JSON: `allowed_zones`) to limit write operations (`AppendRecords`,
`SetRecords`, `DeleteRecords`) to the listed zones. Empty means no
restriction. This is a client-side safeguard against misconfiguration, not a
security boundary.

## Example

Here's a minimal example of how to get all DNS records for zone.

```go
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
```

## Test

The file `provisioner_test.go` contains an end-to-end test suite, using the
original IONOS API service (i.e. no test doubles - be careful). To run the
tests:

```console
$ export LIBDNS_IONOS_TEST_ZONE=mydomain.org
$ export LIBDNS_IONOS_TEST_TOKEN=aaaaaaaaaaa.bbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
$ go  test -v
=== RUN   Test_checkZoneAllowed
=== RUN   Test_checkZoneAllowed/empty_list_allows_all
=== RUN   Test_checkZoneAllowed/exact_match
=== RUN   Test_checkZoneAllowed/trailing_dot_in_zone
=== RUN   Test_checkZoneAllowed/trailing_dot_in_list
=== RUN   Test_checkZoneAllowed/case_insensitive
=== RUN   Test_checkZoneAllowed/not_listed
=== RUN   Test_checkZoneAllowed/subzone_not_matched
--- PASS: Test_checkZoneAllowed (0.00s)
    --- PASS: Test_checkZoneAllowed/empty_list_allows_all (0.00s)
    --- PASS: Test_checkZoneAllowed/exact_match (0.00s)
    --- PASS: Test_checkZoneAllowed/trailing_dot_in_zone (0.00s)
    --- PASS: Test_checkZoneAllowed/trailing_dot_in_list (0.00s)
    --- PASS: Test_checkZoneAllowed/case_insensitive (0.00s)
    --- PASS: Test_checkZoneAllowed/not_listed (0.00s)
    --- PASS: Test_checkZoneAllowed/subzone_not_matched (0.00s)
=== RUN   Test_WritesRejectDisallowedZone
--- PASS: Test_WritesRejectDisallowedZone (0.00s)
=== RUN   Test_ConfigCompatible
--- PASS: Test_ConfigCompatible (0.00s)
=== RUN   Test_AppendRecords
=== RUN   Test_AppendRecords/testcase_0
=== RUN   Test_AppendRecords/testcase_1
=== RUN   Test_AppendRecords/testcase_2
--- PASS: Test_AppendRecords (3.40s)
    --- PASS: Test_AppendRecords/testcase_0 (1.41s)
    --- PASS: Test_AppendRecords/testcase_1 (0.87s)
    --- PASS: Test_AppendRecords/testcase_2 (1.12s)
=== RUN   Test_DeleteRecords
--- PASS: Test_DeleteRecords (1.28s)
=== RUN   Test_DeleteRecordsWillNotDeleteWithoutName
--- PASS: Test_DeleteRecordsWillNotDeleteWithoutName (0.05s)
=== RUN   Test_DeleteRecordsRejectsZoneNotAllowed
--- PASS: Test_DeleteRecordsRejectsZoneNotAllowed (0.00s)
=== RUN   Test_GetRecords
--- PASS: Test_GetRecords (1.77s)
=== RUN   Test_UpdateRecords
--- PASS: Test_UpdateRecords (1.68s)
PASS
ok  	github.com/libdns/ionos	8.190s
```

## Author

Original Work (C) Copyright 2020 by matthiasng (based on https://github.com/libdns/hetzner),
this version (C) Copyright 2021 by Jan Delgado (github.com/jandelgado).

License: MIT
