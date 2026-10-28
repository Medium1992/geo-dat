// Command geo-dat converts sing-box rule-set sources (the "sing" branch of
// MetaCubeX/meta-rules-dat) into Xray geosite.dat / geoip.dat / asn.dat.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Domain.Type values from Xray's geodat.proto.
const (
	typeSubstr = 0
	typeRegex  = 1
	typeDomain = 2
	typeFull   = 3
)

type ruleSet struct {
	Rules []map[string]json.RawMessage `json:"rules"`
}

type domain struct {
	typ   int
	value string
	attrs map[string]bool
}

func main() {
	src := flag.String("src", "sing", "path to a checkout of the sing branch")
	out := flag.String("out", "publish", "output directory")
	flag.Parse()

	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatal(err)
	}
	if err := buildGeoSite(filepath.Join(*src, "geo", "geosite"), filepath.Join(*out, "geosite.dat")); err != nil {
		log.Fatal(err)
	}
	if err := buildGeoIP(filepath.Join(*src, "geo", "geoip"), filepath.Join(*out, "geoip.dat")); err != nil {
		log.Fatal(err)
	}
	if err := buildGeoIP(filepath.Join(*src, "asn"), filepath.Join(*out, "asn.dat")); err != nil {
		log.Fatal(err)
	}
}

// listJSON returns rule-set names (file names without .json), sorted.
func listJSON(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			names = append(names, strings.TrimSuffix(e.Name(), ".json"))
		}
	}
	sort.Strings(names)
	return names, nil
}

// readRuleSet merges all rules of a file into one key -> values map.
func readRuleSet(path string) (map[string][]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var rs ruleSet
	if err := json.Unmarshal(data, &rs); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	merged := map[string][]string{}
	for _, rule := range rs.Rules {
		for key, raw := range rule {
			var values []string
			if err := json.Unmarshal(raw, &values); err != nil {
				// sing-box allows a single string instead of a list.
				var single string
				if err := json.Unmarshal(raw, &single); err != nil {
					return nil, fmt.Errorf("%s: key %q: unsupported value", path, key)
				}
				values = []string{single}
			}
			merged[key] = append(merged[key], values...)
		}
	}
	return merged, nil
}

func readDomains(path string) ([]*domain, error) {
	rs, err := readRuleSet(path)
	if err != nil {
		return nil, err
	}
	var domains []*domain
	for key, values := range rs {
		for _, v := range values {
			switch key {
			case "domain":
				domains = append(domains, &domain{typ: typeFull, value: v})
			case "domain_suffix":
				if rest, ok := strings.CutPrefix(v, "."); ok {
					// ".example.com" matches subdomains only.
					domains = append(domains, &domain{typ: typeRegex, value: `\.` + regexp.QuoteMeta(rest) + `$`})
				} else {
					domains = append(domains, &domain{typ: typeDomain, value: v})
				}
			case "domain_keyword":
				domains = append(domains, &domain{typ: typeSubstr, value: v})
			case "domain_regex":
				domains = append(domains, &domain{typ: typeRegex, value: v})
			default:
				log.Printf("warn: %s: skipping unsupported key %q", path, key)
			}
		}
	}
	return domains, nil
}

func buildGeoSite(dir, outPath string) error {
	names, err := listJSON(dir)
	if err != nil {
		return err
	}

	// "google@cn" is not a separate list in Xray: its domains become the
	// "cn" attribute of entries in "google", selectable as geosite:google@cn.
	bases := map[string][]*domain{}
	attrFiles := map[string][]string{}
	for _, name := range names {
		base, attr, ok := strings.Cut(name, "@")
		if ok {
			attrFiles[base] = append(attrFiles[base], attr)
			continue
		}
		if bases[base], err = readDomains(filepath.Join(dir, name+".json")); err != nil {
			return err
		}
	}
	for base, attrs := range attrFiles {
		for _, attr := range attrs {
			tagged, err := readDomains(filepath.Join(dir, base+"@"+attr+".json"))
			if err != nil {
				return err
			}
			index := map[string]*domain{}
			for _, d := range bases[base] {
				index[fmt.Sprint(d.typ, d.value)] = d
			}
			for _, t := range tagged {
				d, ok := index[fmt.Sprint(t.typ, t.value)]
				if !ok {
					d = t
					bases[base] = append(bases[base], d)
				}
				if d.attrs == nil {
					d.attrs = map[string]bool{}
				}
				d.attrs[strings.ToLower(attr)] = true
			}
		}
	}

	codes := make([]string, 0, len(bases))
	for base := range bases {
		codes = append(codes, base)
	}
	sort.Strings(codes)

	return writeList(outPath, len(codes), func(i int) []byte {
		var entry []byte
		entry = appendString(entry, 1, strings.ToUpper(codes[i]))
		for _, d := range bases[codes[i]] {
			entry = appendBytes(entry, 2, encodeDomain(d))
		}
		return entry
	})
}

func encodeDomain(d *domain) []byte {
	var b []byte
	if d.typ != 0 {
		b = appendTag(b, 1, 0)
		b = appendVarint(b, uint64(d.typ))
	}
	b = appendString(b, 2, d.value)
	attrs := make([]string, 0, len(d.attrs))
	for a := range d.attrs {
		attrs = append(attrs, a)
	}
	sort.Strings(attrs)
	for _, a := range attrs {
		var attr []byte
		attr = appendString(attr, 1, a)
		attr = appendTag(attr, 2, 0) // bool_value = true
		attr = appendVarint(attr, 1)
		b = appendBytes(b, 3, attr)
	}
	return b
}

func buildGeoIP(dir, outPath string) error {
	names, err := listJSON(dir)
	if err != nil {
		return err
	}
	return writeListErr(outPath, len(names), func(i int) ([]byte, error) {
		path := filepath.Join(dir, names[i]+".json")
		rs, err := readRuleSet(path)
		if err != nil {
			return nil, err
		}
		var entry []byte
		entry = appendString(entry, 1, strings.ToUpper(names[i]))
		for key, values := range rs {
			if key != "ip_cidr" {
				log.Printf("warn: %s: skipping unsupported key %q", path, key)
				continue
			}
			for _, v := range values {
				prefix, err := parsePrefix(v)
				if err != nil {
					return nil, fmt.Errorf("%s: %w", path, err)
				}
				var cidr []byte
				cidr = appendBytes(cidr, 1, prefix.Addr().AsSlice())
				cidr = appendTag(cidr, 2, 0)
				cidr = appendVarint(cidr, uint64(prefix.Bits()))
				entry = appendBytes(entry, 2, cidr)
			}
		}
		return entry, nil
	})
}

func parsePrefix(s string) (netip.Prefix, error) {
	if !strings.Contains(s, "/") {
		addr, err := netip.ParseAddr(s)
		if err != nil {
			return netip.Prefix{}, err
		}
		return netip.PrefixFrom(addr, addr.BitLen()), nil
	}
	p, err := netip.ParsePrefix(s)
	if err != nil {
		return netip.Prefix{}, err
	}
	return p.Masked(), nil
}

func writeList(path string, n int, entry func(int) []byte) error {
	return writeListErr(path, n, func(i int) ([]byte, error) { return entry(i), nil })
}

// writeListErr streams a GeoSiteList / GeoIPList: repeated field 1 of entries.
func writeListErr(path string, n int, entry func(int) ([]byte, error)) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	w := bufio.NewWriterSize(f, 1<<20)
	for i := 0; i < n; i++ {
		e, err := entry(i)
		if err != nil {
			f.Close()
			return err
		}
		if _, err := w.Write(appendBytes(nil, 1, e)); err != nil {
			f.Close()
			return err
		}
	}
	if err := w.Flush(); err != nil {
		f.Close()
		return err
	}
	log.Printf("%s: %d entries", path, n)
	return f.Close()
}

func appendVarint(b []byte, v uint64) []byte {
	for v >= 0x80 {
		b = append(b, byte(v)|0x80)
		v >>= 7
	}
	return append(b, byte(v))
}

func appendTag(b []byte, field, wireType int) []byte {
	return appendVarint(b, uint64(field<<3|wireType))
}

func appendBytes(b []byte, field int, v []byte) []byte {
	b = appendTag(b, field, 2)
	b = appendVarint(b, uint64(len(v)))
	return append(b, v...)
}

func appendString(b []byte, field int, v string) []byte {
	return appendBytes(b, field, []byte(v))
}
