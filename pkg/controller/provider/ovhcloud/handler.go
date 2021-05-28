/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved. h file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 *
 */

package ovhcloud

import (
	"context"
	"fmt"
	"strings"

	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/ovh/go-ovh/ovh"

	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
)

// Handler is the main DNSHandler struct.
type Handler struct {
	provider.ZoneCache
	provider.DefaultDNSHandler

	access *access
	config *provider.DNSHandlerConfig
	ctx    context.Context
}

var _ provider.DNSHandler = &Handler{}


// NewHandler constructs a new DNSHandler object.
func NewHandler(config *provider.DNSHandlerConfig) (provider.DNSHandler, error) {
	authConfig, err := readAuthConfig(config)
	if err != nil {
		return nil, err
	}

	client, err := createOvhcloudClient(config.Logger, authConfig)
	if err != nil {
		return nil, err
	}

	h := &Handler{
		DefaultDNSHandler: provider.NewDefaultDNSHandler(TYPE_CODE),
		config:            config,
		ctx:               config.Context,
		access:	           &access{
			client: client,
			metrics: config.Metrics,
			rateLimiter: h.config.RateLimiter,
		}
	}

	h.ZoneCache, err = provider.NewZoneCache(config.CacheConfig, config.Metrics, nil, h.getZones, h.getZoneState)
	if err != nil {
		return nil, err
	}

	return h, nil
}

type clientAuthConfig struct {
	Endpoint          string
	ApplicationKey    string
	ApplicationSecret string
	ConsumerKey       string
}

func readAuthConfig(c *provider.DNSHandlerConfig) (*clientAuthConfig, error) {
	endpoint, err := c.GetRequiredProperty("OVH_ENDPOINT")
	if err != nil {
		return nil, err
	}
	ak, err := c.GetRequiredProperty("OVH_APPLICATION_KEY")
	if err != nil {
		return nil, err
	}
	as, err := c.GetRequiredProperty("OVH_APPLICATION_SECRET")
	if err != nil {
		return nil, err
	}
	ck, err := c.GetRequiredProperty("OVH_CONSUMER_KEY")
	if err != nil {
		return nil, err
	}

	authConfig := clientAuthConfig{
		Endpoint          : endpoint,
		ApplicationKey    : ak,
		ApplicationSecret : as,
		ConsumerKey       : ck,
	}

	return &authConfig, nil
}

func createOvhcloudClient(logger logger.LogContext, clientAuthConfig *clientAuthConfig) (*ovh.Client, error) {
	validEndpoint := false

	ovhEndpoints := [7]string{
		ovh.OvhEU,
		ovh.OvhCA,
		ovh.OvhUS,
		ovh.KimsufiEU,
		ovh.KimsufiCA,
		ovh.SoyoustartEU,
		ovh.SoyoustartCA
	}

	for _, e := range ovhEndpoints {
		if ovh.Endpoints[c.Endpoint] == e {
			validEndpoint = true
		}
	}

	if !validEndpoint {
		return nil, fmt.Errorf("%s must be one of %#v endpoints\n", c.Endpoint, ovh.Endpoints)
	}

	client, err := ovh.NewClient(
		c.Endpoint,
		c.ApplicationKey,
		c.ApplicationSecret,
		c.ConsumerKey,
	)

	if err != nil {
		return nil, fmt.Errorf("Error getting ovh client: %q\n", err)
	}
	return client
}

// Release releases the zone cache.
func (h *Handler) Release() {
	h.cache.Release()
}

// GetZones returns a list of hosted zones from the cache.
func (h *Handler) GetZones() (provider.DNSHostedZones, error) {
	return h.cache.GetZones()
}

func (h *Handler) getZones(cache provider.ZoneCache) (provider.DNSHostedZones, error) {
	zones, err := h.access.getZones()
	if err != nil {
		return nil, fmt.Errorf("listing DNS zones failed. Details: %s", err)
	}

	hostedZones := provider.DNSHostedZones{}
	for _, z := range zones {
		forwarded := []string{}
		records, err := h.access.getRecordSets(z, "", "NS")
		if err != nil {
			return nil, fmt.Errorf("listing DNS zone records failed for zone %s. Details: %s", z, err)
		}

		for _, r := range records {
			name := fmt.Sprintf("%s.%s", r.Subdomain, r.Zone)
			if name != z.Name {
				forwarded = append(forwarded, dns.NormalizeHostname(name))
			}
		}

		hostedZone := provider.NewDNSHostedZone(
			h.ProviderType(),
			z.Name,
			dns.NormalizeHostname(z.Name),
			z.Name,
			forwarded,
			false,
		)

		hostedZones = append(hostedZones, hostedZone)
	}

	return hostedZones, nil
}

// GetZoneState returns the state for a given zone.
func (h *Handler) GetZoneState(zone provider.DNSHostedZone) (provider.DNSZoneState, error) {
	return h.cache.GetZoneState(zone)
}

func (h *Handler) getZoneState(zone provider.DNSHostedZone, cache provider.ZoneCache) (provider.DNSZoneState, error) {
	dnssets := dns.DNSSets{}

	recordSetHandler := func(recordSet *recordsets.RecordSet) error {
		switch recordSet.Type {
		case dns.RS_A, dns.RS_CNAME, dns.RS_TXT:
			rs := dns.NewRecordSet(recordSet.Type, int64(recordSet.TTL), nil)
			for _, record := range recordSet.Records {
				value := record
				if recordSet.Type == dns.RS_CNAME {
					value = dns.NormalizeHostname(value)
				}
				rs.Add(&dns.Record{Value: value})
			}
			dnssets.AddRecordSetFromProvider(recordSet.Name, rs)
		}
		return nil
	}

	h.config.RateLimiter.Accept()
	if err := h.client.ForEachRecordSet(zone.Id(), recordSetHandler); err != nil {
		return nil, fmt.Errorf("Listing DNS zones failed for %s. Details: %s", zone.Id(), err.Error())
	}

	return provider.NewDNSZoneState(dnssets), nil
}

func (h *Handler) ReportZoneStateConflict(zone provider.DNSHostedZone, err error) bool {
	return h.cache.ReportZoneStateConflict(zone, err)
}

// ExecuteRequests applies a given change request to a given hosted zone.
func (h *Handler) ExecuteRequests(logger logger.LogContext, zone provider.DNSHostedZone, state provider.DNSZoneState, reqs []*provider.ChangeRequest) error {
	err := h.executeRequests(logger, zone, state, reqs)
	h.cache.ApplyRequests(logger, err, zone, reqs)
	return err
}


func (h *Handler) ExecuteRequests(logger logger.LogContext, zone provider.DNSHostedZone, state provider.DNSZoneState, reqs []*provider.ChangeRequest) error {
	err := h.executeRequests(logger, zone, state, reqs)
	h.cache.ApplyRequests(logger, err, zone, reqs)
	return err
}

func (h *Handler) executeRequests(logger logger.LogContext, zone provider.DNSHostedZone, state provider.DNSZoneState, reqs []*provider.ChangeRequest) error {
	if h.config.DryRun {
		logger.Infof("no changes in dryrun mode for OVHcloud")
		return nil
	}

	updated := false
	for _, r := range reqs {
		name, rset := dns.MapToProvider(req.Type, dnsset, this.zone.Domain())
		name = dns.AlignHostname(name)
		if len(rset.Records) == 0 {
			return
		}

		this.Infof("%s %s record set %s[%s]: %s(%d)", action, rset.Type, name, this.zone.Id(), rset.RecordString(), rset.TTL)
		for i, r := range rset.Records {
			updated = true
			h.config.RateLimiter.Accept()
			switch r.Action {
			case provider.R_CREATE:
				if err := access.createRecordSet(zone.Id(), name, r.Value, rset.Type, rset.TTL); err != nil {
					return nil, fmt.Errorf("Create DNS zone record failed for %s. Details: %s", zone.Id(), err)
				}
			case provider.R_UPDATE:
				if err := access.updateRecordSet(zone.Id(), name, r.Value, rset.Type, rset.TTL); err != nil {
					return nil, fmt.Errorf("Update DNS zone record failed for %s. Details: %s", zone.Id(), err)
				}
			case provider.R_DELETE:
				if err := access.deleteRecordSet(zone.Id(), name, r.Value, rset.Type); err != nil {
					return nil, fmt.Errorf("Delete DNS zone record failed for %s. Details: %s", zone.Id(), err)
				}
			}
		}
	}

	if updated {
		h.config.RateLimiter.Accept()
		if err := access.refreshZone(zone.Id()); err != nil {
			return nil, fmt.Errorf("Refesh DNS zone failed for %s. Details: %s", zone.Id(), err)
		}
	}
	return nil
}
