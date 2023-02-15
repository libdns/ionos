// libdns client for IONOS DNS API
package ionos

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/libdns/libdns"
)

const (
	APIEndpoint = "https://api.hosting.ionos.com/dns/v1"
)

type getAllZonesResponse struct {
	Zones []zoneDescriptor
}

type zoneDescriptor struct {
	Name string `json:"name"`
	ID   string `json:"id"`
	Type string `json:"type"`
}

type getZoneResponse struct {
	ID      string       `json:"id"`
	Name    string       `json:"name"`
	Type    string       `json:"type"`
	Records []zoneRecord `json:"records"`
}

type zoneRecord struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	RootName   string `json:"rootName"`
	Type       string `json:"type"`
	Content    string `json:"content"`
	ChangeDate string `json:"changeDate"`
	TTL        int    `json:"ttl"`
	Prio       int    `json:"prio"`
	Disabled   bool   `json:"disabled"`
}

type record struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Content  string `json:"content"`
	TTL      *int   `json:"ttl,omitempty"`
	Prio     int    `json:"prio"`
	Disabled bool   `json:"disabled,omitempty"` // TODO default=true
}

// IONOS does not accept TTL values < 60, and returns status 400. If the
// TTL is 0, we leave the field empty, by setting the struct value to nil.
func optTTL(ttl float64) *int {
	var intTTL *int
	if ttl > 0 {
		tmp := int(ttl)
		intTTL = &tmp
	}
	return intTTL
}

func doRequest(token string, request *http.Request) ([]byte, error) {
	request.Header.Add("X-API-Key", token)
	request.Header.Add("Content-Type", "application/json")

	client := &http.Client{} // no timeout set because request is w/ context
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("%s (%d)", http.StatusText(response.StatusCode), response.StatusCode)
	}
	return ioutil.ReadAll(response.Body)
}

// GET /v1/zones
func getAllZones(ctx context.Context, token string) (getAllZonesResponse, error) {
	uri := fmt.Sprintf("%s/zones", APIEndpoint)
	req, err := http.NewRequestWithContext(ctx, "GET", uri, nil)
	data, err := doRequest(token, req)

	if err != nil {
		return getAllZonesResponse{}, err
	}

	// parse top-level JSON array
	zones := make([]zoneDescriptor, 0)
	err = json.Unmarshal(data, &zones)
	return getAllZonesResponse{zones}, err
}

// findZoneDescriptor finds the zoneDescriptor for the named zoned in all zones
func findZoneDescriptor(ctx context.Context, token string, zoneName string) (zoneDescriptor, error) {
	allZones, err := getAllZones(ctx, token)
	if err != nil {
		return zoneDescriptor{}, err
	}
	for _, zone := range allZones.Zones {
		if zone.Name == zoneName {
			return zone, nil
		}
	}
	return zoneDescriptor{}, fmt.Errorf("zone not found")
}

// getZone reads a zone by it's IONOS zoneID
// /v1/zones/{zoneId}
func getZone(ctx context.Context, token string, zoneID string) (getZoneResponse, error) {
	uri := fmt.Sprintf("%s/zones/%s", APIEndpoint, zoneID)
	req, err := http.NewRequestWithContext(ctx, "GET", uri, nil)
	data, err := doRequest(token, req)
	var result getZoneResponse
	if err != nil {
		return result, err
	}

	err = json.Unmarshal(data, &result)
	return result, err
}

// findRecordInZone searches all records in the given zone for a record with
// the given name and type and returns this record on success
func findRecordInZone(ctx context.Context, token, zoneName, name, typ string) (zoneRecord, error) {
	zoneResp, err := getZoneByName(ctx, token, zoneName)
	if err != nil {
		return zoneRecord{}, err
	}

	for _, r := range zoneResp.Records {
		if r.Name == name && r.Type == typ {
			return r, nil
		}
	}
	return zoneRecord{}, fmt.Errorf("record not found")
}

// getZoneByName reads a zone by it's zone name, requiring 2 REST calls to
// the IONOS API
func getZoneByName(ctx context.Context, token, zoneName string) (getZoneResponse, error) {
	zoneDes, err := findZoneDescriptor(ctx, token, zoneName)
	if err != nil {
		return getZoneResponse{}, err
	}
	return getZone(ctx, token, zoneDes.ID)
}

// getAllRecords returns all records from the given zone
func getAllRecords(ctx context.Context, token string, zoneName string) ([]libdns.Record, error) {
	zoneResp, err := getZoneByName(ctx, token, zoneName)
	if err != nil {
		return nil, err
	}
	records := []libdns.Record{}
	for _, r := range zoneResp.Records {
		records = append(records, libdns.Record{
			ID:   r.ID,
			Type: r.Type,
			// libdns Name is partially qualified, relative to zone
			Name:  libdns.RelativeName(r.Name, zoneResp.Name),
			Value: r.Content,
			TTL:   time.Duration(r.TTL) * time.Second,
		})
	}
	return records, nil
}

// createRecord creates a DNS record in the given zone
// POST /v1/zones/{zoneId}/records
func createRecord(ctx context.Context, token string, zoneName string, r libdns.Record) (libdns.Record, error) {
	zoneResp, err := getZoneByName(ctx, token, zoneName)
	if err != nil {
		return libdns.Record{}, err
	}

	reqData := []record{
		{Type: r.Type,
			// IONOS: Name is fully qualified
			Name:    libdns.AbsoluteName(r.Name, zoneName),
			Content: r.Value,
			TTL:     optTTL(r.TTL.Seconds()),
		}}

	reqBuffer, err := json.Marshal(reqData)
	if err != nil {
		return libdns.Record{}, err
	}

	uri := fmt.Sprintf("%s/zones/%s/records", APIEndpoint, zoneResp.ID)
	req, err := http.NewRequestWithContext(ctx, "POST", uri, bytes.NewBuffer(reqBuffer))
	if err != nil {
		return libdns.Record{}, err
	}

	// as result of the POST, a zoneDescriptor array is returned
	data, err := doRequest(token, req)
	if err != nil {
		return libdns.Record{}, err
	}
	zones := make([]zoneDescriptor, 0)
	if err = json.Unmarshal(data, &zones); err != nil {
		return libdns.Record{}, err
	}

	if len(zones) != 1 {
		return libdns.Record{}, fmt.Errorf("unexpected response from create record (size mismatch)")
	}

	return libdns.Record{
		ID:   zones[0].ID,
		Type: r.Type,
		// always return partially qualified name, relative to zone for libdns
		Name:  libdns.RelativeName(unFQDN(r.Name), zoneName),
		Value: r.Value,
		TTL:   r.TTL,
	}, nil
}

// DELETE /v1/zones/{zoneId}/records/{recordId}
func deleteRecord(ctx context.Context, token, zoneName string, record libdns.Record) error {
	zoneResp, err := getZoneByName(ctx, token, zoneName)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "DELETE",
		fmt.Sprintf("%s/zones/%s/records/%s", APIEndpoint, zoneResp.ID, record.ID), nil)
	if err != nil {
		return err
	}
	_, err = doRequest(token, req)
	return err
}

// /v1/zones/{zoneId}/records/{recordId}
func updateRecord(ctx context.Context, token string, zone string, r libdns.Record) (libdns.Record, error) {
	zoneDes, err := getZoneByName(ctx, token, zone)
	if err != nil {
		return libdns.Record{}, err
	}

	reqData := record{
		Type:    r.Type,
		Name:    libdns.AbsoluteName(r.Name, zone),
		Content: r.Value,
		TTL:     optTTL(r.TTL.Seconds()),
	}

	reqBuffer, err := json.Marshal(reqData)
	if err != nil {
		return libdns.Record{}, err
	}

	req, err := http.NewRequestWithContext(ctx, "PUT",
		fmt.Sprintf("%s/zones/%s/records/%s", APIEndpoint, zoneDes.ID, r.ID),
		bytes.NewBuffer(reqBuffer))

	if err != nil {
		return libdns.Record{}, err
	}

	_, err = doRequest(token, req)

	return libdns.Record{
		ID:    r.ID,
		Type:  r.Type,
		Name:  r.Name,
		Value: r.Value,
		TTL:   time.Duration(r.TTL) * time.Second,
	}, err
}

func createOrUpdateRecord(ctx context.Context, token string, zone string, r libdns.Record) (libdns.Record, error) {
	if r.ID == "" {
		return createRecord(ctx, token, zone, r)
	}
	return updateRecord(ctx, token, zone, r)
}
