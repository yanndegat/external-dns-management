/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package source

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"
)

func (this *sourceReconciler) exclude(dns string) bool {
	if this.excluded.Contains(dns) {
		return true
	}
	for d := range this.excluded {
		if strings.HasPrefix(d, "*.") {
			d = d[2:]
			i := strings.Index(dns, ".")
			if i >= 0 {
				if d == dns[i+1:] {
					return true
				}
			}
		}
	}
	return false
}

func (this *sourceReconciler) getDNSInfo(logger logger.LogContext, obj resources.Object, s DNSSource, current *DNSCurrentState) (*DNSInfo, bool, error) {
	obj = this.enrichAnnotations(obj)

	if !this.classes.IsResponsibleFor(logger, obj) {
		return nil, false, nil
	}

	annos := obj.GetAnnotations()
	current.AnnotatedNames = utils.StringSet{}
	current.AnnotatedNames.AddAllSplittedSelected(annos[DNS_ANNOTATION], utils.StandardNonEmptyStringElement)

	info, err := s.GetDNSInfo(logger, obj, current)
	if info != nil && info.Names != nil {
		for d := range info.Names {
			if this.exclude(d) {
				info.Names.Remove(d)
			}
		}
	}
	if err != nil {
		return info, true, err
	}
	if info == nil {
		return nil, true, nil
	}
	if info.TTL == nil {
		a := annos[TTL_ANNOTATION]
		if a != "" {
			ttl, err := strconv.ParseInt(a, 10, 64)
			if err != nil {
				return info, true, fmt.Errorf("invalid TTL: %s", err)
			}
			if ttl != 0 {
				info.TTL = &ttl
			}
		}
	}
	if info.Interval == nil {
		a := annos[PERIOD_ANNOTATION]
		if a != "" {
			interval, err := strconv.ParseInt(a, 10, 64)
			if err != nil {
				return info, true, fmt.Errorf("invalid check Interval: %s", err)
			}
			if interval != 0 {
				info.Interval = &interval
			}
		}
	}
	return info, true, nil
}

func (this *sourceReconciler) enrichAnnotations(obj resources.Object) resources.Object {
	addons := this.annotations.GetInfoFor(obj.ClusterKey())
	if len(addons) > 0 {
		obj = obj.DeepCopy()
		annos := obj.GetAnnotations()

		annotatedNames := utils.StringSet{}
		annotatedNames.AddAllSplittedSelected(annos[DNS_ANNOTATION], utils.StandardNonEmptyStringElement)

		for k, v := range addons {
			if k == DNS_ANNOTATION {
				annotatedNames.AddAllSplittedSelected(v, utils.StandardNonEmptyStringElement)
				logger.Infof("adding dns names by annotation injection: %s", v)
			} else {
				if old, ok := annos[k]; !ok || old != v {
					annos[k] = v
					logger.Infof("using annotation injection: %s=%s", k, v)
				}
			}
		}

		if len(annotatedNames) > 0 {
			annos[DNS_ANNOTATION] = strings.Join(annotatedNames.AsArray(), ",")
		}
		obj.SetAnnotations(annos)
	}
	return obj
}
