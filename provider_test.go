// end-to-end test suite, using the original IONOS API service (i.e. no test
// doubles - be careful). set environment variables
//
//	LIBDNS_IONOS_TEST_TOKEN - API token
//	LIBDNS_IONOS_TEST_ZONE - domain
//
// before running the test.
package ionos_test

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/libdns/ionos"
	"github.com/libdns/libdns"
)

var (
	envToken = ""
	envZone  = ""
	ttl      = time.Duration(120 * time.Second)
)

var letters = []rune("abcdefghijklmnopqrstuvwxyz")

func randSeq(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func cleanupRecords(t *testing.T, p *ionos.Provider, r []libdns.Record) {
	t.Helper()
	_, err := p.DeleteRecords(context.TODO(), envZone, r)
	if err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}
}

func checkExcatlyOneRecordExists(
	t *testing.T,
	records []libdns.Record,
	recordType, name, value string) {

	t.Helper()
	name = strings.ToLower(name)
	found := 0
	for _, r := range records {
		if r.Name == name {
			found++
			if r.Type != recordType || r.Value != value {
				t.Fatalf("expected to find excatly one %s record with name %s and value of %s", recordType, name, value)
			}
		}
	}
	if found != 1 {
		t.Fatalf("expected to find only one record named %s, but found %d", value, found)
	}
}

func checkNoRecordExists(
	t *testing.T,
	records []libdns.Record,
	name string) {

	t.Helper()
	for _, r := range records {
		if r.Name == strings.ToLower(name) {
			t.Fatalf("expected to find no record named %s", r.Name)
		}
	}
}

func containsRecord(probe libdns.Record, records []libdns.Record) *libdns.Record {
	for _, r := range records {
		if r.Name == probe.Name &&
			r.Type == probe.Type &&
			r.Value == probe.Value &&
			r.TTL == probe.TTL {
			return &r
		}
	}
	return nil
}

// Test_AppendRecords creates various records using AppendRecords and checks
// that the response returned is as expected. Records are not read back
// using GetRecords, that's done in Test_GetRecords.
func Test_AppendRecords(t *testing.T) {
	p := &ionos.Provider{AuthAPIToken: envToken}

	prefix := randSeq(32)
	testCases := []struct {
		records  []libdns.Record
		expected []libdns.Record
	}{
		{
			// multiple records
			records: []libdns.Record{
				{Type: "TXT", Name: prefix + "_atest_1", Value: "val_1", TTL: ttl},
				{Type: "TXT", Name: prefix + "_atest_2", Value: "val_2", TTL: 0},
			},
			expected: []libdns.Record{
				{Type: "TXT", Name: prefix + "_atest_1", Value: "val_1", TTL: ttl},
				{Type: "TXT", Name: prefix + "_atest_2", Value: "val_2", TTL: time.Hour},
			},
		},
		{
			// relative name
			records: []libdns.Record{
				{Type: "TXT", Name: prefix + "123.atest", Value: "123", TTL: ttl},
			},
			expected: []libdns.Record{
				{Type: "TXT", Name: prefix + "123.atest", Value: "123", TTL: ttl},
			},
		},
		{
			// A records
			records: []libdns.Record{
				{Type: "A", Name: prefix + "456.atest.", Value: "1.2.3.4", TTL: ttl},
			},
			expected: []libdns.Record{
				{Type: "A", Name: prefix + "456.atest", Value: "1.2.3.4", TTL: ttl},
			},
		},
	}

	for i, c := range testCases {
		t.Run(fmt.Sprintf("testcase %d", i),
			func(t *testing.T) {
				result, err := p.AppendRecords(context.TODO(), envZone, c.records)
				if err != nil {
					t.Fatal(err)
				}
				defer cleanupRecords(t, p, result)

				if len(result) != len(c.expected) {
					t.Fatalf("unexpected number of records created: expected %d != actual %d", len(c.expected), len(result))
				}

				// results are returned in arbitrary order
				for _, r := range c.expected {
					if containsRecord(r, result) == nil {
						t.Fatalf("record %+v was not created", r)
					}
				}
				// each created record must have an ID
				for _, r := range result {
					if r.ID == "" {
						t.Fatalf("no ID set in result %+v", r)
					}
				}
			})
	}
}

func Test_DeleteRecords(t *testing.T) {
	p := &ionos.Provider{AuthAPIToken: envToken}

	// run the test 2 times: first delete with the record ID, second run without
	for _, clearID := range []bool{true, false} {
		t.Run(fmt.Sprintf("clear record.ID=%v", clearID),
			func(t *testing.T) {
				// create a random TXT record
				name := randSeq(32)
				records := []libdns.Record{{Type: "TXT", Name: name, Value: "my record", TTL: ttl}}
				records, err := p.SetRecords(context.TODO(), envZone, records)
				if err != nil {
					t.Fatal(err)
				}
				//defer cleanupRecords(t, p, slices.Clone(records))
				if len(records) != 1 {
					t.Fatalf("expected only 1 record to be created, but got %d", len(records))
				}

				// make sure the record exists in the zone
				allRecords, err := p.GetRecords(context.TODO(), envZone)
				if err != nil {
					t.Fatal(err)
				}
				checkExcatlyOneRecordExists(t, allRecords, "TXT", name, "my record")

				// test with- and without a recordID
				if clearID {
					records[0].ID = ""
				}
				records, err = p.DeleteRecords(context.TODO(), envZone, records)
				if err != nil {
					t.Fatal(err)
				}
				if len(records) != 1 {
					t.Fatalf("expected only 1 record to be deleted, but got %d", len(records))
				}

				// make sure the record is no longer in the zone
				allRecords, err = p.GetRecords(context.TODO(), envZone)
				if err != nil {
					t.Fatal(err)
				}
				checkNoRecordExists(t, allRecords, name)
			})
	}
}

func Test_DeleteRecordsWillNotDeleteWithoutTypeOrNameWhenNoIDisGiven(t *testing.T) {
	p := &ionos.Provider{AuthAPIToken: envToken}

	records := []libdns.Record{
		{ID: "", Type: "TXT", Name: "", Value: "", TTL: ttl},
		{ID: "", Type: "", Name: "X", Value: "", TTL: ttl},
		{ID: "", Type: "", Name: "", Value: "", TTL: ttl}}

	records, err := p.DeleteRecords(context.TODO(), envZone, records)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 {
		t.Fatalf("expected no record to be deleted, but got %d", len(records))
	}

}

// Test_GetRecords creates some records and checks using GetRecords that
// the records are returned as expected
func Test_GetRecords(t *testing.T) {
	p := &ionos.Provider{AuthAPIToken: envToken}

	// create some test records
	prefix := randSeq(32)
	records := []libdns.Record{
		{Type: "TXT", Name: prefix + "_test_1", Value: "val_1", TTL: ttl},
		{Type: "A", Name: prefix + "_test_2", Value: "1.2.3.4", TTL: ttl}}
	created, err := p.AppendRecords(context.TODO(), envZone, records)
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != len(records) {
		t.Fatalf("expected %d records to be created, got %d", len(records), len(created))
	}
	defer cleanupRecords(t, p, created)

	// read all records of the zone and check that our records are contained
	allRecords, err := p.GetRecords(context.TODO(), envZone)
	if len(allRecords) < len(records) {
		t.Fatalf("expected to read at least %d records from zone, but got %d", len(records), len(allRecords))
	}

	for _, r := range created {
		found := containsRecord(r, allRecords)
		if found == nil {
			t.Fatalf("Record %s not found", r.ID)
		}
		if found.ID != r.ID {
			t.Fatalf("Record found but ID differs (%s != %s)", r.ID, found.ID)
		}
	}
}

func Test_UpdateRecords(t *testing.T) {
	p := &ionos.Provider{AuthAPIToken: envToken}

	// run the test 2 times: first delete with the record ID, second run without
	for _, clearID := range []bool{true, false} {
		t.Run(fmt.Sprintf("clear record.ID=%v", clearID),
			func(t *testing.T) {
				// create a random A record
				name := randSeq(32)
				records := []libdns.Record{{Type: "A", Name: name, Value: "1.2.3.4", TTL: ttl}}
				records, err := p.SetRecords(context.TODO(), envZone, records)
				if err != nil {
					t.Fatal(err)
				}
				defer cleanupRecords(t, p, slices.Clone(records))

				if len(records) != 1 {
					t.Fatalf("expected only 1 record to be created, but got %d", len(records))
				}

				// test with- and without a recordID
				if clearID {
					records[0].ID = ""
				}
				records[0].Value = "1.2.3.5"
				records, err = p.SetRecords(context.TODO(), envZone, records)
				if len(records) != 1 {
					t.Fatalf("expected only 1 record to be updated, but got %d", len(records))
				}

				// read all records and check for the expected changes
				records, err = p.GetRecords(context.TODO(), envZone)
				if err != nil {
					t.Fatal(err)
				}
				checkExcatlyOneRecordExists(t, records, "A", name, "1.2.3.5")
			})
	}
}

func TestMain(m *testing.M) {
	envToken = os.Getenv("LIBDNS_IONOS_TEST_TOKEN")
	envZone = os.Getenv("LIBDNS_IONOS_TEST_ZONE")

	if len(envToken) == 0 || len(envZone) == 0 {
		fmt.Println(`Please notice that this test runs agains the public ionos DNS Api, so you sould
never run the test with a zone, used in production.
To run this test, you have to specify 'LIBDNS_IONOS_TEST_TOKEN' and 'LIBDNS_IONOS_TEST_ZONE'.
Example: "LIBDNS_IONOS_TEST_TOKEN="123.456" LIBDNS_IONOS_TEST_ZONE="my-domain.com" go test ./... -v`)
		os.Exit(1)
	}

	os.Exit(m.Run())
}
