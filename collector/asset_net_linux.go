// Copyright 2025 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build linux && !noasset_net

package collector

import (
	"log/slog"
	"strings"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/node_exporter/collector/asset/cmdb"
)

type assetNetCollector struct {
	info   *prometheus.Desc
	cache  assetCache[*cmdb.Net]
	logger *slog.Logger
}

func init() {
	registerCollector("asset_net", defaultEnabled, NewAssetNetCollector)
}

// NewAssetNetCollector returns a collector exposing physical NIC and bond
// identity and topology under siliconflow_asset_*. Only physical NICs and
// bond interfaces are reported (container/K8s virtual interfaces excluded by the
// vendored cmdb collector).
func NewAssetNetCollector(logger *slog.Logger) (Collector, error) {
	return &assetNetCollector{
		info: prometheus.NewDesc(
			prometheus.BuildFQName(assetNamespace, "", "net_info"),
			"A metric with a constant '1' value labeled by NIC identity (physical, bond master, slaves, vendor, driver).",
			[]string{
				assetUUIDLabel, "name", "physical", "master",
				"slaves", "vendor", "driver",
			},
			nil,
		),
		logger: logger,
	}, nil
}

func (c *assetNetCollector) Update(ch chan<- prometheus.Metric) error {
	uuid, err := readAssetUUID()
	if err != nil {
		return err
	}
	n, err := c.cache.get(*assetCacheTTL, func() (*cmdb.Net, error) {
		return cmdb.CollectNet()
	})
	if err != nil {
		return err
	}

	for _, dev := range n.Devices {
		ch <- prometheus.MustNewConstMetric(c.info, prometheus.GaugeValue, 1,
			uuid,
			assetLabel(dev.Name),
			assetBool(dev.Physical),
			assetLabel(dev.Master),
			strings.Join(dev.Slaves, ","),
			assetLabel(dev.Vendor),
			assetLabel(dev.Driver),
		)
	}
	return nil
}
