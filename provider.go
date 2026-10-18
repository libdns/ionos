// libdns implementation for IONOS DNS API.
// IONOS API documentaion: https://developer.hosting.ionos.de/docs/dns
// libdns: https://github.com/libdns/libdns
package ionos

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/libdns/libdns"
)

// Provider implements the libdns interfaces for IONOS
type Provider struct {
	// AuthAPIToken is the IONOS Auth API token -
	// see https://dns.ionos.com/api-docs#section/Authentication/Auth-API-Token
	AuthAPIToken string `json:"auth_api_token"`

	// AllowedZones optionally restricts write operations to the listed zones.
	// Empty means no restriction. Read operations are unaffected.
	AllowedZones []string `json:"allowed_zones,omitempty"`
}

// ErrZoneNotAllowed is returned by write operations on a zone not listed in
// AllowedZones.
var ErrZoneNotAllowed = errors.New("zone is not in the configured allow-list")

// sameZone compares zone names, ignoring case and trailing dots.
func sameZone(a, b string) bool {
	return strings.EqualFold(unFQDN(a), unFQDN(b))
}

// inZone reports whether name is zone or one of its subdomains.
func inZone(name, zone string) bool {
	suffix := "." + strings.ToLower(unFQDN(zone))
	return sameZone(name, zone) || strings.HasSuffix(strings.ToLower(unFQDN(name)), suffix)
}

// checkZoneAllowed returns ErrZoneNotAllowed if AllowedZones is set and
// either zone is not listed or a record name lies outside zone (e.g. an
// absolute name like "x.other.com.").
func (p *Provider) checkZoneAllowed(zone string, records []libdns.Record) error {
	if len(p.AllowedZones) == 0 {
		return nil
	}

	allowed := slices.ContainsFunc(p.AllowedZones, func(z string) bool { return sameZone(z, zone) })
	if !allowed {
		return fmt.Errorf("%w: %q", ErrZoneNotAllowed, zone)
	}

	for _, r := range records {
		name := libdns.AbsoluteName(r.RR().Name, zone)
		if !inZone(name, zone) {
			return fmt.Errorf("%w: record %q outside zone %q", ErrZoneNotAllowed, name, zone)
		}
	}
	return nil
}

// findWritableZone resolves zone for a write operation, enforcing AllowedZones.
func (p *Provider) findWritableZone(
	ctx context.Context,
	zone string,
	records []libdns.Record,
) (zoneDescriptor, error) {
	if err := p.checkZoneAllowed(zone, records); err != nil {
		return zoneDescriptor{}, err
	}
	return p.findZoneByName(ctx, zone)
}

func toIonosRecord(r libdns.Record, zoneName string) record {
	rr := r.RR()
	return record{
		Type:    rr.Type,
		Name:    libdns.AbsoluteName(rr.Name, zoneName),
		Content: rr.Data,
		TTL:     ionosTTL(rr.TTL.Seconds()),
	}
}

func fromIonosRecord(r zoneRecord, zoneName string) (libdns.Record, error) {
	// libdns Name is partially qualified, relative to zone, Ionos absoulte
	name := libdns.RelativeName(r.Name, zoneName) // use r.rootName for zoneName TODO?
	ttl := time.Duration(r.TTL) * time.Second

	switch strings.ToUpper(r.Type) {
	case "MX":
		return libdns.MX{Name: name, TTL: ttl, Target: r.Content, Preference: uint16(r.Prio)}, nil
	case "TXT":
		// IONOS returns TXT records quoted: remove quotes
		text, err := strconv.Unquote(r.Content)
		return libdns.TXT{Name: name, TTL: ttl, Text: text}, err
	default:
		return libdns.RR{
			Name: name,
			TTL:  ttl,
			Type: r.Type,
			Data: r.Content,
		}.Parse()
	}
}

func (p *Provider) findZoneByName(ctx context.Context, zoneName string) (zoneDescriptor, error) {
	// obtain list of all zones
	zones, err := ionosGetAllZones(ctx, p.AuthAPIToken)
	if err != nil {
		return zoneDescriptor{}, fmt.Errorf("get all zones: %w", err)
	}

	// find the desired zone
	for _, zone := range zones.Zones {
		if sameZone(zone.Name, zoneName) {
			return zone, nil
		}
	}
	return zoneDescriptor{}, fmt.Errorf("zone named not found (%s)", zoneName)
}

// GetRecords lists all the records in the zone.
func (p *Provider) GetRecords(ctx context.Context, zoneName string) ([]libdns.Record, error) {
	zoneDes, err := p.findZoneByName(ctx, zoneName)
	if err != nil {
		return nil, fmt.Errorf("find zone: %w", err)
	}

	// obtain list of all records in zone
	zoneResp, err := ionosGetZone(ctx, p.AuthAPIToken, zoneDes.ID, "", "")
	if err != nil {
		return nil, fmt.Errorf("get zone records: %w", err)
	}

	records := make([]libdns.Record, len(zoneResp.Records))
	for i, r := range zoneResp.Records {
		record, err := fromIonosRecord(r, zoneName)
		if err != nil {
			return records, fmt.Errorf("convert record: %w", err)
		}
		records[i] = record
	}
	return records, nil
}

// AppendRecords adds records to the zone. It returns the records that were added.
func (p *Provider) AppendRecords(
	ctx context.Context,
	zone string,
	records []libdns.Record,
) ([]libdns.Record, error) {
	zoneDes, err := p.findWritableZone(ctx, zone, records)
	if err != nil {
		return nil, fmt.Errorf("find zone: %w", err)
	}

	// populate ionos request
	reqs := make([]record, len(records))
	for i, r := range records {
		reqs[i] = toIonosRecord(r, zoneDes.Name)
	}

	newRecords, err := ionosCreateRecords(ctx, p.AuthAPIToken, zoneDes.ID, reqs)
	if err != nil {
		return nil, fmt.Errorf("create records: %w", err)
	}

	// populate libdns response
	results := make([]libdns.Record, len(records))
	for i, r := range newRecords {
		result, err := fromIonosRecord(r, zoneDes.Name)
		if err != nil {
			return results, fmt.Errorf("convert record: %w", err)
		}
		results[i] = result
	}
	return results, nil
}

// DeleteRecords deletes the given records from the zone if they exist in the
// zone and exactly match the input. If the input records do not exist in the
// zone, they are silently ignored. DeleteRecords returns only the the records
// that were deleted, and does not return any records that were provided in the
// input but did not exist in the zone.
//
// DeleteRecords only deletes records from the zone that *exactly* match the
// input records—that is, the name, type, TTL, and value all must be identical
// to a record in the zone for it to be deleted.
//
// As a special case, you may leave any of the fields [libdns.Record.Type],
// [libdns.Record.TTL], or [libdns.Record.Value] empty ("", 0, and ""
// respectively). In this case, DeleteRecords will delete any records that
// match the other fields, regardless of the value of the fields that were left
// empty. Note that this behavior does *not* apply to the [libdns.Record.Name]
// field, which must always be specified.
//
// Note that it is semantically invalid to remove the last “NS” record from a
// zone, so attempting to do is undefined behavior.
//
// Implementations should return struct types defined by this package which
// correspond with the specific RR-type, rather than the [RR] struct, if possible.
//
// Implementations must honor context cancellation and be safe for concurrent
// use.
//
// libdns-ionos notes: we use ionosFindRecordsInZone to filter the records,
// which does not support TTL
func (p *Provider) DeleteRecords(
	ctx context.Context,
	zone string,
	records []libdns.Record,
) ([]libdns.Record, error) {
	zoneDes, err := p.findWritableZone(ctx, zone, records)
	if err != nil {
		return nil, fmt.Errorf("find zone: %w", err)
	}

	// ionos api has no batch-delete, delete one record at a time
	var deleteQueue []libdns.Record // list of record IDs to delete

	for _, r := range records {
		rr := r.RR()
		// safety: avoid deleting the whole zone
		if rr.Type == "" || rr.Name == "" {
			continue
		}

		// search record first to obtain the record ID, which is needed to delete the record
		name := libdns.AbsoluteName(rr.Name, zoneDes.Name)
		existing, err := ionosFindRecordsInZone(ctx, p.AuthAPIToken, zoneDes.ID, rr.Type, name)
		// TODO according to libdns spec, we need to also match for TTL and
		// value of the record
		if err != nil {
			return nil, fmt.Errorf("find record for deletion: %w", err)
		}
		for _, found := range existing {
			result, err := fromIonosRecord(found, zoneDes.Name)
			if err != nil {
				return deleteQueue, fmt.Errorf("convert record: %w", err)
			}
			if err := ionosDeleteRecord(ctx, p.AuthAPIToken, zoneDes.ID, found.ID); err != nil {
				return deleteQueue, fmt.Errorf("delete record %+v, %w", found, err)
			}
			deleteQueue = append(deleteQueue, result)
		}
	}

	return deleteQueue, nil
}

func (p *Provider) createOrUpdateRecord(
	ctx context.Context,
	zoneDes zoneDescriptor,
	r libdns.Record,
) (libdns.Record, error) {
	rr := r.RR()
	// before we create a new record, make sure there is no existing record
	// of same (type, name). In this case we only update the record
	name := libdns.AbsoluteName(rr.Name, zoneDes.Name)
	existing, err := ionosFindRecordsInZone(ctx, p.AuthAPIToken, zoneDes.ID, rr.Type, name)
	if err == nil {
		if len(existing) != 1 {
			return r, fmt.Errorf("unexpected number of records during delete, expected 1, found %d", len(existing))
		}
		err := ionosUpdateRecord(ctx, p.AuthAPIToken, zoneDes.ID, existing[0].ID, toIonosRecord(r, zoneDes.Name))
		if err != nil {
			return r, fmt.Errorf("update found record: %w", err)
		}
		return r, nil
	}

	created, err := ionosCreateRecords(ctx, p.AuthAPIToken, zoneDes.ID, []record{toIonosRecord(r, zoneDes.Name)})
	if err != nil {
		return r, fmt.Errorf("create new record: %w", err)
	}
	if len(created) != 1 {
		return r, fmt.Errorf("expected one record to be created, got %d", len(created))
	}
	return fromIonosRecord(created[0], zoneDes.Name)
}

// SetRecords sets the records in the zone, either by updating existing records
// or creating new ones. It returns the updated records.
func (p *Provider) SetRecords(ctx context.Context, zone string, records []libdns.Record) ([]libdns.Record, error) {
	var res []libdns.Record

	zoneDes, err := p.findWritableZone(ctx, zone, records)
	if err != nil {
		return nil, fmt.Errorf("find zone: %w", err)
	}

	for _, r := range records {
		newRecord, err := p.createOrUpdateRecord(ctx, zoneDes, r)
		if err != nil {
			return res, err
		}
		res = append(res, newRecord)
	}
	return res, nil
}

func (p *Provider) ListZones(ctx context.Context) ([]libdns.Zone, error) {
	zones, err := ionosGetAllZones(ctx, p.AuthAPIToken)
	if err != nil {
		return []libdns.Zone{}, fmt.Errorf("get all zones: %w", err)
	}
	result := make([]libdns.Zone, len(zones.Zones))
	for i, zone := range zones.Zones {
		result[i].Name = zone.Name
	}
	return result, nil
}

// unFQDN trims any trailing "." from fqdn. IONOS's API does not use FQDNs.
func unFQDN(fqdn string) string {
	return strings.TrimSuffix(fqdn, ".")
}

// Interface guards
var (
	_ libdns.RecordGetter   = (*Provider)(nil)
	_ libdns.RecordAppender = (*Provider)(nil)
	_ libdns.RecordSetter   = (*Provider)(nil)
	_ libdns.RecordDeleter  = (*Provider)(nil)
	_ libdns.ZoneLister     = (*Provider)(nil)
)