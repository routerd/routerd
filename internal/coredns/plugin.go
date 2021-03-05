/*
Copyright 2021 The routerd authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package coredns

import (
	"context"
	"fmt"
	"sync"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/file"
	"github.com/coredns/coredns/request"
	"github.com/go-logr/logr"
	"github.com/miekg/dns"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	dnsv1alpha1 "routerd.net/routerd/apis/dns/v1alpha1"
)

const pluginName = "routerd"

type routerd struct {
	Next plugin.Handler

	client client.Client
	log    logr.Logger

	zones     map[string]*file.Zone
	zoneNames plugin.Zones
	zoneMux   sync.RWMutex
}

var _ plugin.Handler = (*routerd)(nil)

func newRouterdPlugin(
	client client.Client, log logr.Logger,
) *routerd {
	return &routerd{
		client: client,
		log:    log,

		zones: map[string]*file.Zone{},
	}
}

func (p *routerd) Name() string { return pluginName }

func (p *routerd) ServeDNS(
	ctx context.Context, w dns.ResponseWriter, req *dns.Msg,
) (int, error) {
	dnsRequest := request.Request{W: w, Req: req}

	var (
		zoneName string
		zone     *file.Zone
	)
	func() {
		p.zoneMux.RLock()
		defer p.zoneMux.RUnlock()

		zoneName = p.zoneNames.Matches(dnsRequest.Name())
		zone = p.zones[zoneName]
	}()
	if len(zoneName) == 0 {
		// we don't handle this zone
		return plugin.NextOrFailure(pluginName, p.Next, ctx, w, req)
	}
	if zone == nil {
		// we should handle this zone, but we have no data for it.
		p.log.Info("matching zone without zone data!")
		return dns.RcodeServerFailure, nil
	}

	m := &dns.Msg{}
	m.SetReply(req)
	m.Authoritative = true

	var result file.Result
	m.Answer, m.Ns, m.Extra, result = zone.Lookup(ctx, dnsRequest, dnsRequest.Name())

	switch result {
	case file.Success, file.NoData:
		// noop
	case file.NameError:
		m.Rcode = dns.RcodeNameError
	case file.Delegation:
		m.Authoritative = false
	case file.ServerFailure:
		return dns.RcodeServerFailure, nil
	}

	return dns.RcodeSuccess, w.WriteMsg(m)
}

// Reconcile runs for every DNS change and reindexes all Zones and
// RecordSets of the server.
func (p *routerd) Reconcile(ctx context.Context, req ctrl.Request) (
	result ctrl.Result, err error) {
	// ctrl.Request may reference either a Zone or RecordSet,
	// so we ignore it and rebuild our whole config from cache.

	zoneList := &dnsv1alpha1.ZoneList{}
	if err := p.client.List(ctx, zoneList); err != nil {
		return result, err
	}

	var (
		zoneNames []string
		zones     = map[string]*file.Zone{}
	)
	for _, zone := range zoneList.Items {
		zoneName := zone.Name + "."
		z := file.NewZone(zoneName, "")
		zones[zoneName] = z
		zoneNames = append(zoneNames, zoneName)

		// SOA
		rr, err := soaRecord(zoneName, &zone)
		if err != nil {
			return result, fmt.Errorf("creating SOA record: %w", err)
		}
		if err := z.Insert(rr); err != nil {
			return result, fmt.Errorf("insert SOA record: %w", err)
		}

		// Add other records
		recordSetList := &dnsv1alpha1.RecordSetList{}
		if err := p.client.List(ctx, recordSetList); err != nil {
			return result, fmt.Errorf("listing RecordSets: %w", err)
		}

		for _, recordSet := range recordSetList.Items {
			values := recordSet.Values()
			if len(values) == 0 {
				// skip record sets without records
				continue
			}

			for _, v := range values {
				rfc1035 := fmt.Sprintf(
					"%s %d IN %s %s",
					recordSet.DNSName,
					int(recordSet.TTL.Duration.Seconds()),
					recordSet.GetType(),
					v,
				)
				rr, err := dns.NewRR(rfc1035)
				if err != nil {
					return result, fmt.Errorf(
						"failed to create DNS record from RFC1035 string %q: %w",
						rfc1035, err)
				}
				if err := z.Insert(rr); err != nil {
					return result, fmt.Errorf("insert record: %w", err)
				}
			}
		}
	}

	// update served zone data
	p.zoneMux.Lock()
	defer p.zoneMux.Unlock()
	p.zoneNames = zoneNames
	p.zones = zones
	return result, nil
}

func (p *routerd) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&dnsv1alpha1.Zone{}).
		Watches(
			&source.Kind{Type: &dnsv1alpha1.RecordSet{}},
			&handler.EnqueueRequestForObject{},
		).Complete(p)
}

func soaRecord(zoneName string, zone *dnsv1alpha1.Zone) (dns.RR, error) {
	soa := zone.SOA
	soaString := fmt.Sprintf(
		"%s %s %d %d %d %d %d",
		soa.Master,
		soa.Admin,
		soa.Serial,
		int(soa.Refresh.Duration.Seconds()),
		int(soa.Retry.Duration.Seconds()),
		int(soa.Expire.Duration.Seconds()),
		int(soa.NegativeTTL.Duration.Seconds()))
	rfc1035 := fmt.Sprintf(
		"%s %d IN SOA %s",
		zoneName,
		int(soa.TTL.Duration.Seconds()),
		soaString)
	return dns.NewRR(rfc1035)
}
