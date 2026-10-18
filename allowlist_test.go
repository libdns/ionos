package ionos

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/libdns/libdns"
)

func Test_checkZoneAllowed(t *testing.T) {
	tests := []struct {
		name    string
		allowed []string
		zone    string
		wantErr bool
	}{
		{"empty list allows all", nil, "example.com", false},
		{"exact match", []string{"example.com"}, "example.com", false},
		{"trailing dot in zone", []string{"example.com"}, "example.com.", false},
		{"trailing dot in list", []string{"example.com."}, "example.com", false},
		{"case insensitive", []string{"Example.COM"}, "example.com", false},
		{"not listed", []string{"example.com"}, "other.com", true},
		{"subzone not matched", []string{"example.com"}, "sub.example.com", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := Provider{AllowedZones: tc.allowed}
			err := p.checkZoneAllowed(tc.zone, nil)
			if errors.Is(err, ErrZoneNotAllowed) != tc.wantErr {
				t.Fatalf("got err=%v, wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

// Record names must stay inside the allowed zone, absolute names included.
func Test_checkZoneAllowedRecordNames(t *testing.T) {
	tests := []struct {
		name    string
		allowed []string
		record  string
		wantErr bool
	}{
		{"relative", []string{"example.com"}, "_acme-challenge", false},
		{"apex", []string{"example.com"}, "@", false},
		{"absolute inside", []string{"example.com"}, "_acme-challenge.Example.com.", false},
		{"absolute outside", []string{"example.com"}, "_acme-challenge.victim.com.", true},
		{"suffix trick", []string{"example.com"}, "x.notexample.com.", true},
		{"no allow-list", nil, "_acme-challenge.victim.com.", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := Provider{AllowedZones: tc.allowed}
			recs := []libdns.Record{libdns.TXT{Name: tc.record, Text: "x"}}
			err := p.checkZoneAllowed("example.com", recs)
			if errors.Is(err, ErrZoneNotAllowed) != tc.wantErr {
				t.Fatalf("got err=%v, wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

// Write operations on a disallowed zone fail before any API call, so a
// dummy token suffices.
func Test_WritesRejectDisallowedZone(t *testing.T) {
	p := &Provider{AuthAPIToken: "dummy", AllowedZones: []string{"example.com"}}
	ctx := context.Background()
	recs := []libdns.Record{libdns.TXT{Name: "test", Text: "x"}}

	if _, err := p.AppendRecords(ctx, "other.com", recs); !errors.Is(err, ErrZoneNotAllowed) {
		t.Errorf("AppendRecords: got %v, want ErrZoneNotAllowed", err)
	}
	if _, err := p.SetRecords(ctx, "other.com", recs); !errors.Is(err, ErrZoneNotAllowed) {
		t.Errorf("SetRecords: got %v, want ErrZoneNotAllowed", err)
	}
	if _, err := p.DeleteRecords(ctx, "other.com", recs); !errors.Is(err, ErrZoneNotAllowed) {
		t.Errorf("DeleteRecords: got %v, want ErrZoneNotAllowed", err)
	}
}

// Existing configs without allowed_zones must still decode and keep the
// unrestricted behavior.
func Test_ConfigCompatible(t *testing.T) {
	var p Provider
	if err := json.Unmarshal([]byte(`{"auth_api_token":"abc"}`), &p); err != nil {
		t.Fatal(err)
	}
	if p.AuthAPIToken != "abc" || len(p.AllowedZones) != 0 {
		t.Fatalf("unexpected provider: %+v", p)
	}

	if err := json.Unmarshal([]byte(`{"auth_api_token":"abc","allowed_zones":["example.com"]}`), &p); err != nil {
		t.Fatal(err)
	}
	if len(p.AllowedZones) != 1 || p.AllowedZones[0] != "example.com" {
		t.Fatalf("unexpected allowed_zones: %+v", p.AllowedZones)
	}
}
