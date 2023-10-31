package maxminddb

import (
	"fmt"
	"net/netip"
	"strings"
	"sync/atomic"

	"github.com/oschwald/maxminddb-golang"
)

const (
	dataTypeSingGeoIP       = "sing-geoip"
	dataTypeMetaGeoIP       = "Meta-geoip0"
	dataTypeGeoLite2Country = "GeoLite2Country"
)

type Reader struct {
	dataType string
	reader   *maxminddb.Reader
	n        *atomic.Int32
}

func OpenMaxmindDBReader(path string, dataType string) (*Reader, error) {
	switch dataType {
	case "sing-geoip", "sing", "sing-box":
		dataType = dataTypeSingGeoIP
	case "meta-geoip", "meta", "clash.meta":
		dataType = dataTypeMetaGeoIP
	case "geolite2-country", "":
		dataType = dataTypeGeoLite2Country
	default:
		return nil, fmt.Errorf("unknown database type %s", dataType)
	}
	db, err := maxminddb.Open(path)
	if err != nil {
		return nil, err
	}
	if db.Metadata.DatabaseType != dataType {
		db.Close()
		return nil, fmt.Errorf("unknown database type %s", db.Metadata.DatabaseType)
	}
	reader := &Reader{
		dataType: dataType,
		reader:   db,
		n:        &atomic.Int32{},
	}
	reader.n.Add(1)
	return reader, nil
}

type Country struct {
	Continent struct {
		Names     map[string]string `maxminddb:"names"`
		Code      string            `maxminddb:"code"`
		GeoNameID uint              `maxminddb:"geoname_id"`
	} `maxminddb:"continent"`
	Country struct {
		Names             map[string]string `maxminddb:"names"`
		IsoCode           string            `maxminddb:"iso_code"`
		GeoNameID         uint              `maxminddb:"geoname_id"`
		IsInEuropeanUnion bool              `maxminddb:"is_in_european_union"`
	} `maxminddb:"country"`
	RegisteredCountry struct {
		Names             map[string]string `maxminddb:"names"`
		IsoCode           string            `maxminddb:"iso_code"`
		GeoNameID         uint              `maxminddb:"geoname_id"`
		IsInEuropeanUnion bool              `maxminddb:"is_in_european_union"`
	} `maxminddb:"registered_country"`
	RepresentedCountry struct {
		Names             map[string]string `maxminddb:"names"`
		IsoCode           string            `maxminddb:"iso_code"`
		Type              string            `maxminddb:"type"`
		GeoNameID         uint              `maxminddb:"geoname_id"`
		IsInEuropeanUnion bool              `maxminddb:"is_in_european_union"`
	} `maxminddb:"represented_country"`
	Traits struct {
		IsAnonymousProxy    bool `maxminddb:"is_anonymous_proxy"`
		IsSatelliteProvider bool `maxminddb:"is_satellite_provider"`
	} `maxminddb:"traits"`
}

func (r *Reader) Clone() *Reader {
	r.n.Add(1)
	return r
}

func (r *Reader) Lookup(addr netip.Addr) []string {
	switch r.dataType {
	case dataTypeSingGeoIP:
		var data string
		err := r.reader.Lookup(addr.AsSlice(), &data)
		if err == nil {
			return []string{data}
		}
	case dataTypeMetaGeoIP:
		var data []string
		err := r.reader.Lookup(addr.AsSlice(), &data)
		if err == nil {
			return data
		}
	case dataTypeGeoLite2Country:
		var data Country
		err := r.reader.Lookup(addr.AsSlice(), &data)
		if err == nil {
			var code string
			if data.Country.IsoCode != "" {
				code = strings.ToLower(data.Country.IsoCode)
			} else if data.RegisteredCountry.IsoCode != "" {
				code = strings.ToLower(data.RegisteredCountry.IsoCode)
			} else if data.RepresentedCountry.IsoCode != "" {
				code = strings.ToLower(data.RepresentedCountry.IsoCode)
			} else if data.Continent.Code != "" {
				code = strings.ToLower(data.Continent.Code)
			}
			if code != "" {
				return []string{code}
			}
		}
	}
	return nil
}

func (r *Reader) Close() error {
	if r.n.Add(-1) == 0 {
		return r.reader.Close()
	}
	return nil
}
