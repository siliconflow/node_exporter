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

//go:build linux && !noasset_cpu

package collector

import (
	"log/slog"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/node_exporter/collector/asset/cmdb"
)

type assetCPUCollector struct {
	info          *prometheus.Desc
	deviceCores   *prometheus.Desc
	deviceThreads *prometheus.Desc
	deviceCache   *prometheus.Desc
	cache         assetCache[*cmdb.CPU]
	logger        *slog.Logger
}

func init() {
	registerCollector("asset_cpu", defaultEnabled, NewAssetCPUCollector)
}

// NewAssetCPUCollector returns a collector exposing per-socket CPU topology
// and identity under siliconflow_asset_*. Machine-level aggregate counts
// (sockets/cores/threads) are no longer emitted; consumers derive them from
// the per-socket device metrics.
func NewAssetCPUCollector(logger *slog.Logger) (Collector, error) {
	return &assetCPUCollector{
		info: prometheus.NewDesc(
			prometheus.BuildFQName(assetNamespace, "", "cpu_info"),
			"A metric with a constant '1' value labeled by per-socket CPU identity (model name, vendor id).",
			[]string{assetUUIDLabel, "socket", "model_name", "vendor_id"}, nil,
		),
		deviceCores: prometheus.NewDesc(
			prometheus.BuildFQName(assetNamespace, "", "cpu_device_cores"),
			"Number of physical cores on a single socket.",
			[]string{assetUUIDLabel, "socket"}, nil,
		),
		deviceThreads: prometheus.NewDesc(
			prometheus.BuildFQName(assetNamespace, "", "cpu_device_threads"),
			"Number of logical threads on a single socket.",
			[]string{assetUUIDLabel, "socket"}, nil,
		),
		deviceCache: prometheus.NewDesc(
			prometheus.BuildFQName(assetNamespace, "", "cpu_device_cache_kb"),
			"CPU cache size of a single socket in kilobytes.",
			[]string{assetUUIDLabel, "socket"}, nil,
		),
		logger: logger,
	}, nil
}

func (c *assetCPUCollector) Update(ch chan<- prometheus.Metric) error {
	uuid, err := readAssetUUID()
	if err != nil {
		return err
	}
	cpu, err := c.cache.get(*assetCacheTTL, func() (*cmdb.CPU, error) {
		return cmdb.CollectCPU()
	})
	if err != nil {
		return err
	}

	for i, dev := range cpu.Devices {
		socket := strconv.Itoa(i)
		ch <- prometheus.MustNewConstMetric(c.info, prometheus.GaugeValue, 1,
			uuid, socket, assetLabel(dev.ModelName), assetLabel(dev.VendorID))
		ch <- prometheus.MustNewConstMetric(c.deviceCores, prometheus.GaugeValue, float64(dev.Cores), uuid, socket)
		ch <- prometheus.MustNewConstMetric(c.deviceThreads, prometheus.GaugeValue, float64(dev.Threads), uuid, socket)
		ch <- prometheus.MustNewConstMetric(c.deviceCache, prometheus.GaugeValue, float64(dev.CacheKB), uuid, socket)
	}
	return nil
}
