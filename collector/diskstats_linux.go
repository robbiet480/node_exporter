// +build !nodiskstats

package collector

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/log"
)

const (
	procDiskStats = "/proc/diskstats"
	diskSubsystem = "disk"
)

var (
	ignoredDevices = flag.String("collector.diskstats.ignored-devices", "^(ram|loop|fd|(h|s|v|xv)d[a-z])\\d+$", "Regexp of devices to ignore for diskstats.")
)

type diskstatsCollector struct {
	ignoredDevicesPattern *regexp.Regexp
	metrics               []prometheus.Collector
}

func init() {
	Factories["diskstats"] = NewDiskstatsCollector
}

// Takes a prometheus registry and returns a new Collector exposing
// disk device stats.
func NewDiskstatsCollector() (Collector, error) {
	var diskLabelNames = []string{"device"}

	return &diskstatsCollector{
		ignoredDevicesPattern: regexp.MustCompile(*ignoredDevices),
		// Docs from https://www.kernel.org/doc/Documentation/iostats.txt
		metrics: []prometheus.Collector{
			prometheus.NewCounterVec(
				prometheus.CounterOpts{
					Namespace: Namespace,
					Subsystem: diskSubsystem,
					Name:      "reads_completed",
					Help:      "The total number of reads completed successfully.",
				},
				diskLabelNames,
			),
			prometheus.NewCounterVec(
				prometheus.CounterOpts{
					Namespace: Namespace,
					Subsystem: diskSubsystem,
					Name:      "reads_merged",
					Help:      "The number of reads merged. See https://www.kernel.org/doc/Documentation/iostats.txt.",
				},
				diskLabelNames,
			),
			prometheus.NewCounterVec(
				prometheus.CounterOpts{
					Namespace: Namespace,
					Subsystem: diskSubsystem,
					Name:      "sectors_read",
					Help:      "The total number of sectors read successfully.",
				},
				diskLabelNames,
			),
			prometheus.NewCounterVec(
				prometheus.CounterOpts{
					Namespace: Namespace,
					Subsystem: diskSubsystem,
					Name:      "read_time_ms",
					Help:      "The total number of milliseconds spent by all reads.",
				},
				diskLabelNames,
			),
			prometheus.NewCounterVec(
				prometheus.CounterOpts{
					Namespace: Namespace,
					Subsystem: diskSubsystem,
					Name:      "writes_completed",
					Help:      "The total number of writes completed successfully.",
				},
				diskLabelNames,
			),
			prometheus.NewCounterVec(
				prometheus.CounterOpts{
					Namespace: Namespace,
					Subsystem: diskSubsystem,
					Name:      "writes_merged",
					Help:      "The number of writes merged. See https://www.kernel.org/doc/Documentation/iostats.txt.",
				},
				diskLabelNames,
			),
			prometheus.NewCounterVec(
				prometheus.CounterOpts{
					Namespace: Namespace,
					Subsystem: diskSubsystem,
					Name:      "sectors_written",
					Help:      "The total number of sectors written successfully.",
				},
				diskLabelNames,
			),
			prometheus.NewCounterVec(
				prometheus.CounterOpts{
					Namespace: Namespace,
					Subsystem: diskSubsystem,
					Name:      "write_time_ms",
					Help:      "This is the total number of milliseconds spent by all writes.",
				},
				diskLabelNames,
			),
			prometheus.NewGaugeVec(
				prometheus.GaugeOpts{
					Namespace: Namespace,
					Subsystem: diskSubsystem,
					Name:      "io_now",
					Help:      "The number of I/Os currently in progress.",
				},
				diskLabelNames,
			),
			prometheus.NewCounterVec(
				prometheus.CounterOpts{
					Namespace: Namespace,
					Subsystem: diskSubsystem,
					Name:      "io_time_ms",
					Help:      "Milliseconds spent doing I/Os.",
				},
				diskLabelNames,
			),
			prometheus.NewCounterVec(
				prometheus.CounterOpts{
					Namespace: Namespace,
					Subsystem: diskSubsystem,
					Name:      "io_time_weighted",
					Help:      "The weighted # of milliseconds spent doing I/Os. See https://www.kernel.org/doc/Documentation/iostats.txt.",
				},
				diskLabelNames,
			),
		},
	}, nil
}

func (c *diskstatsCollector) Update(ch chan<- prometheus.Metric) (err error) {
	diskStats, err := getDiskStats()
	if err != nil {
		return fmt.Errorf("couldn't get diskstats: %s", err)
	}

	for dev, stats := range diskStats {
		if c.ignoredDevicesPattern.MatchString(dev) {
			log.Debugf("Ignoring device: %s", dev)
			continue
		}

		if len(stats) != len(c.metrics) {
			return fmt.Errorf("invalid line for %s for %s", procDiskStats, dev)
		}

		for k, value := range stats {
			v, err := strconv.ParseFloat(value, 64)
			if err != nil {
				return fmt.Errorf("invalid value %s in diskstats: %s", value, err)
			}

			if counter, ok := c.metrics[k].(*prometheus.CounterVec); ok {
				counter.WithLabelValues(dev).Set(v)
			} else if gauge, ok := c.metrics[k].(*prometheus.GaugeVec); ok {
				gauge.WithLabelValues(dev).Set(v)
			} else {
				return fmt.Errorf("unexpected collector %d", k)
			}
		}
	}
	for _, c := range c.metrics {
		c.Collect(ch)
	}
	return err
}

func getDiskStats() (map[string]map[int]string, error) {
	file, err := os.Open(procDiskStats)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return parseDiskStats(file)
}

func parseDiskStats(r io.Reader) (map[string]map[int]string, error) {
	var (
		diskStats = map[string]map[int]string{}
		scanner   = bufio.NewScanner(r)
	)

	for scanner.Scan() {
		parts := strings.Fields(string(scanner.Text()))
		if len(parts) < 4 { // we strip major, minor and dev
			return nil, fmt.Errorf("invalid line in %s: %s", procDiskStats, scanner.Text())
		}
		dev := parts[2]
		diskStats[dev] = map[int]string{}
		for i, v := range parts[3:] {
			diskStats[dev][i] = v
		}
	}

	return diskStats, nil
}
