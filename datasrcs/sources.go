// Copyright 2017 Jeff Foley. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package datasrcs

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/OWASP/Amass/v3/config"
	"github.com/OWASP/Amass/v3/net/dns"
	"github.com/OWASP/Amass/v3/net/http"
	"github.com/OWASP/Amass/v3/requests"
	"github.com/OWASP/Amass/v3/semaphore"
	"github.com/OWASP/Amass/v3/stringset"
	"github.com/OWASP/Amass/v3/systems"
	"github.com/PuerkitoBio/goquery"
	"github.com/geziyor/geziyor"
	"github.com/geziyor/geziyor/client"
)

var (
	subRE       = dns.AnySubdomainRegex()
	maxCrawlSem = semaphore.NewSimpleSemaphore(50)
	nameStripRE = regexp.MustCompile(`^u[0-9a-f]{4}|20|22|25|2b|2f|3d|3a|40`)
)

// GetAllSources returns a slice of all data source services, initialized and ready.
func GetAllSources(sys systems.System) []requests.Service {
	srvs := []requests.Service{
		NewAlienVault(sys),
		NewArchiveIt(sys),
		NewArchiveToday(sys),
		NewArquivo(sys),
		NewAsk(sys),
		NewBaidu(sys),
		NewBinaryEdge(sys),
		NewBing(sys),
		NewBufferOver(sys),
		NewCensys(sys),
		NewCertSpotter(sys),
		NewCIRCL(sys),
		NewCommonCrawl(sys),
		NewCrtsh(sys),
		NewDNSDB(sys),
		NewDNSDumpster(sys),
		NewDNSTable(sys),
		NewDogpile(sys),
		NewEntrust(sys),
		NewExalead(sys),
		NewGitHub(sys),
		NewGoogle(sys),
		NewGoogleCT(sys),
		NewHackerOne(sys),
		NewHackerTarget(sys),
		NewIPToASN(sys),
		NewIPv4Info(sys),
		NewLoCArchive(sys),
		NewMnemonic(sys),
		NewNetcraft(sys),
		NewNetworksDB(sys),
		NewOpenUKArchive(sys),
		NewPassiveTotal(sys),
		NewPastebin(sys),
		NewRADb(sys),
		NewRapidDNS(sys),
		NewRiddler(sys),
		NewRobtex(sys),
		NewSiteDossier(sys),
		NewSecurityTrails(sys),
		NewShadowServer(sys),
		NewShodan(sys),
		NewSpyse(sys),
		NewSublist3rAPI(sys),
		NewTeamCymru(sys),
		NewThreatCrowd(sys),
		NewTwitter(sys),
		NewUKGovArchive(sys),
		NewUmbrella(sys),
		NewURLScan(sys),
		NewViewDNS(sys),
		NewVirusTotal(sys),
		NewWayback(sys),
		NewWhoisXML(sys),
		NewYahoo(sys),
	}

	// Filtering in-place: https://github.com/golang/go/wiki/SliceTricks
	i := 0
	for _, s := range srvs {
		if shouldEnable(s.String(), sys.Config()) {
			srvs[i] = s
			i++
		}
	}
	srvs = srvs[:i]
	return srvs
}

func shouldEnable(srvName string, cfg *config.Config) bool {
	include := !cfg.SourceFilter.Include

	for _, name := range cfg.SourceFilter.Sources {
		if strings.EqualFold(srvName, name) {
			include = cfg.SourceFilter.Include
			break
		}
	}

	return include
}

// Clean up the names scraped from the web.
func cleanName(name string) string {
	var err error

	name, err = strconv.Unquote("\"" + strings.TrimSpace(name) + "\"")
	if err == nil {
		name = subRE.FindString(name)
	}

	name = strings.ToLower(name)
	for {
		name = strings.Trim(name, "-.")

		if i := nameStripRE.FindStringIndex(name); i != nil {
			name = name[i[1]:]
		} else {
			break
		}
	}

	return name
}

func crawl(ctx context.Context, baseURL, baseDomain, subdomain, domain string) ([]string, error) {
	results := stringset.New()

	cfg := ctx.Value(requests.ContextConfig).(*config.Config)
	if cfg == nil {
		return results.Slice(), errors.New("crawler error: Failed to obtain the config from Context")
	}

	maxCrawlSem.Acquire(1)
	defer maxCrawlSem.Release(1)

	re := cfg.DomainRegex(domain)
	if re == nil {
		return results.Slice(), fmt.Errorf("crawler error: Failed to obtain regex object for: %s", domain)
	}

	start := fmt.Sprintf("%s/%s/%s", baseURL, strconv.Itoa(time.Now().Year()), subdomain)
	geziyor.NewGeziyor(&geziyor.Options{
		AllowedDomains:              []string{baseDomain},
		StartURLs:                   []string{start},
		Timeout:                     30 * time.Second,
		RobotsTxtDisabled:           true,
		UserAgent:                   http.UserAgent,
		RequestDelayRandomize:       true,
		LogDisabled:                 true,
		ConcurrentRequests:          3,
		ConcurrentRequestsPerDomain: 3,
		ParseFunc: func(g *geziyor.Geziyor, r *client.Response) {
			r.HTMLDoc.Find("a").Each(func(i int, s *goquery.Selection) {
				if href, ok := s.Attr("href"); ok {
					if sub := re.FindString(r.JoinURL(href)); sub != "" {
						if cn := cleanName(sub); cn != "" {
							results.Insert(cn)
						}
					}
				}
			})
		},
	}).Start()

	return results.Slice(), nil
}
