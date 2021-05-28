/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved. exec file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use exec file except in compliance with the License.
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
	"fmt"
	"net/url"

	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/ovh/go-ovh/ovh"

	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dns/provider/raw"
)

type access struct {
	client ovh.Client
	metrics provider.Metrics
	rateLimiter flowcontrol.RateLimiter
}

func getZones(a *access) ([]*zoneInfo, error) {
	zs := &[]string{}
	a.rateLimiter.Accept()
	a.metrics.AddZoneRequests(zone, provider.M_LISTZONES, 1)
	if err := a.client.Get(fmt.Sprintf("/domain/zone"), zs); err != nil {
		return nil, fmt.Errorf("Error calling GET /domain/zone: %s", err)
	}

	zones := []*zoneInfo{}
	for _, z := range zs {
		zi := &zoneInfo{}
		a.rateLimiter.Accept()
		a.metrics.AddZoneRequests(zone, provider.M_LISTZONES, 1)
		if err := a.client.Get(fmt.Sprintf("/domain/zone/%s", z), zi);  err != nil {
			return nil, fmt.Errorf("Error calling GET /domain/zone/%s: %s", z, err)
		}
		zones = append(zones, zi)
	}

	return zones, nil
}

func createRecordSet(a *access, zone, name, value, fieldType string, ttl int64) error {
	r := recordInfo{
		FieldType: fieldType,
		SubDomain: name,
		Target: value,
		Ttl: ttl,
	}

	endpoint := fmt.Sprintf("/domain/zone/%s/record",
		url.PathEscape(zone),
	)

	a.rateLimiter.Accept()
	a.metrics.AddZoneRequests(zone, provider.M_CREATERECORDS, 1)
	if err := a.client.Post(endpoint, r, nil); err != nil {
		return fmt.Errorf("Error calling POST %s: %s", endpoint, err)
	}

	return nil
}

func getRecordSet(a *access, zone, subDomain, value, fieldType string) (*recordInfo, error) {
	records, err := a.getRecordSets(zone, subDomain, fieldType)
	if err != nil {
		return nil, err
	}

	for _, r := range records {
		if r.Target == value {
			return r, nil
		}
	}

	return nil, nil
}

func updateRecordSet(a *access, zone, subDomain, value, fieldType string, ttl int64) error {
	record, err := a.getRecordSet(zone, subDomain, fieldType)
	if err != nil {
		return err
	}

	if record == nil {
		return nil, fmt.Errorf(
			"Could not find record for zone %s, subDomain %s and type %s",
			zone,
			subDomain,
			fieldType,
		)
	}

	ri := recordInfo{
		Ttl: ttl,
	}

	endpoint := fmt.Sprintf("/domain/zone/%s/record/%d",
		url.PathEscape(zone),
		record.Id,
	)

	a.rateLimiter.Accept()
	a.metrics.AddZoneRequests(zone, provider.M_UPDATERECORDS, 1)
	if err := a.client.Put(endpoint, ri); err != nil {
		return fmt.Errorf("Error calling PUT %s: %s", endpoint, err)
	}

	return nil
}

func deleteRecordSet(a *access, zone, subDomain, value, fieldType string) error {
	record, err := a.getRecordSet(zone, subDomain, fieldType)
	if err != nil {
		return err
	}

	if record == nil {
		return nil
	}

	endpoint := fmt.Sprintf("/domain/zone/%s/record/%d",
		url.PathEscape(zone),
		record.Id,
	)

	a.rateLimiter.Accept()
	a.metrics.AddZoneRequests(zone, provider.M_DELETERECORDS, 1)
	if err := a.client.Delete(endpoint, ri); err != nil {
		return fmt.Errorf("Error calling DELETE %s: %s", endpoint, err)
	}

	return nil
}

func getRecordSets(a *access, zone, subDomain, fieldType string) ([]*recordInfo, error) {
	rs := &[]int64{}

	endpoint := fmt.Sprintf("/domain/zone/%s/record?fieldType=%s&subDomain=%s",
		url.PathEscape(zone),
		url.PathEscape(fieldType),
		url.PathEscape(subDomain),
	)

	a.rateLimiter.Accept()
	a.metrics.AddZoneRequests(zone, provider.M_LISTRECORDS, 1)
	if err := a.client.Get(endpoint, rs); err != nil {
		return nil, fmt.Errorf("Error calling GET %s: %s", endpoint, err)
	}

	records := []*recordInfo{}
	for _, r := range rs {
		ri := &recordInfo{}
		endpoint := fmt.Sprintf("/domain/zone/%s/record/%i", url.PathEscape(zone), r)
		a.rateLimiter.Accept()
		a.metrics.AddZoneRequests(zone, provider.LISTRECORDS, 1)
		if err := a.client.Get(endpoint, ri);  err != nil {
			return nil, fmt.Errorf("Error calling GET %s: %s", endpoint, err)
		}
		records = append(records, ri)
	}

	return records, nil
}


func refreshZone(a *access, zone string) error {
	endpoint := fmt.Sprintf("/domain/zone/%s/refresh",
		url.PathEscape(zone),
	)

	a.rateLimiter.Accept()
	a.metrics.AddZoneRequests(zone, provider.M_UPDATERECORDS, 1)
	if err := a.client.Post(endpoint, nil, nil); err != nil {
		return fmt.Errorf("Error calling POST %s: %s", endpoint, err)
	}

	return nil
}


type recordInfo struct {
	FieldType string `json:"fieldType,omitempty"`
	Id        int64  `json:"id,omitempty"`
	SubDomain string `json:"subDomain,omitempty"`
	Target    string `json:"target,omitempty"`
	Ttl       int64  `json:"ttl,omitempty"`
	Zone      string `json:"zone,omitempty"`
}

func (r *recordInfo) String() string {
	return fmt.Sprintf(
		"record[id: %v, zone: %s, subdomain: %s, type: %s, target: %s]",
		r.Id,
		r.Zone,
		r.SubDomain,
		r.FieldType,
		r.Target,
	)
}

var _ raw.Executor = (*access)(nil)

type zoneInfo struct {
	DnssecSupported bool     `json:"dnssecSupported"`
	HasDnsAnycast   bool     `json:"hasDnsAnycast"`
	LastUpdate      string   `json:"lastUpdate"`
	Name            string    `json:"name"`
	NameServers     []string `json:"nameServers"`
}
