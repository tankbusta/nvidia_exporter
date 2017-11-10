package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

// VecInfo stores the prometheus help and labels to
type VecInfo struct {
	help   string
	labels []string
}

var (
	// DefaultNamespace is the base namespace used by the exporter
	DefaultNamespace = "nvml"
	// unexported variables below
	listenAddress string
	metricsPath   string

	gaugeMetrics = map[string]*VecInfo{
		"power_watts": &VecInfo{
			help:   "Power Usage of an NVIDIA GPU in Watts",
			labels: []string{"device_id", "device_uuid", "device_name"},
		},
		"fan_speed": &VecInfo{
                        help:   "Device Fan Speed in Percent of Maximum",
                        labels: []string{"device_id", "device_uuid", "device_name"},
                },
		"gpu_percent": &VecInfo{
			help:   "Percent of GPU Utilized",
			labels: []string{"device_id", "device_uuid", "device_name"},
		},
		"memory_free": &VecInfo{
			help:   "Number of bytes free in the GPU Memory",
			labels: []string{"device_id", "device_uuid", "device_name"},
		},
		"memory_total": &VecInfo{
			help:   "Total bytes of the GPU's memory",
			labels: []string{"device_id", "device_uuid", "device_name"},
		},
		"memory_used": &VecInfo{
			help:   "Total number of bytes used in the GPU Memory",
			labels: []string{"device_id", "device_uuid", "device_name"},
		},
		"memory_percent": &VecInfo{
			help:   "Percent of GPU Memory Utilized",
			labels: []string{"device_id", "device_uuid", "device_name"},
		},
		"temperature_fahrenheit": &VecInfo{
			help:   "GPU Temperature in Fahrenheit",
			labels: []string{"device_id", "device_uuid", "device_name"},
		},
		"temperature_celsius": &VecInfo{
			help:   "GPU Temperature in Celsius",
			labels: []string{"device_id", "device_uuid", "device_name"},
		},
	}
)

// Exporter TODO
type Exporter struct {
	mutex sync.RWMutex

	up     prometheus.Gauge
	gauges map[string]*prometheus.GaugeVec

	devices []Device
}

// NewExporter TODO
func NewExporter() (*Exporter, error) {
	devices, err := GetDevices()
	if err != nil {
		return nil, err
	}

	exp := &Exporter{
		gauges: make(map[string]*prometheus.GaugeVec, len(gaugeMetrics)),
		up: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: DefaultNamespace,
			Name:      "up",
			Help:      "Were the NVML queries successful?",
		}),
		devices: devices,
	}

	for name, info := range gaugeMetrics {
		exp.gauges[name] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: DefaultNamespace,
			Name:      name,
			Help:      info.help,
		}, info.labels)
	}

	return exp, nil
}

// Describe describes all the metrics ever exported by the nvml/nvidia exporter.
// It implements prometheus.Collector.
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- e.up.Desc()

	for _, vec := range e.gauges {
		vec.Describe(ch)
	}
}

// GetTelemetryFromNVML collects device telemetry from all NVIDIA GPUs connected to this machine
func (e *Exporter) GetTelemetryFromNVML() {
	var (
		gpuMem                    *NVMLMemory
		powerUsage                int
		fanSpeed		  int
		gpuPercent, memoryPercent int
		err                       error
		tempF, tempC              int
	)

	for idx, device := range e.devices {
		id := strconv.Itoa(idx)
		if gpuPercent, memoryPercent, err = device.GetUtilization(); err != nil {
			goto ErrorFetching
		}
		e.gauges["gpu_percent"].WithLabelValues(id, device.DeviceUUID, device.DeviceName).Set(float64(gpuPercent))
		e.gauges["memory_percent"].WithLabelValues(id, device.DeviceUUID, device.DeviceName).Set(float64(memoryPercent))

		if tempF, tempC, err = device.GetTemperature(); err != nil {
			goto ErrorFetching
		}
		e.gauges["temperature_celsius"].WithLabelValues(id, device.DeviceUUID, device.DeviceName).Set(float64(tempC))
		e.gauges["temperature_fahrenheit"].WithLabelValues(id, device.DeviceUUID, device.DeviceName).Set(float64(tempF))

		if powerUsage, err = device.GetPowerUsage(); err != nil {
			goto ErrorFetching
		}
		e.gauges["power_watts"].WithLabelValues(id, device.DeviceUUID, device.DeviceName).Set(float64(powerUsage))

		if fanSpeed, err = device.GetFanSpeed(); err != nil {
                        goto ErrorFetching
                }
		e.gauges["fan_speed"].WithLabelValues(id, device.DeviceUUID, device.DeviceName).Set(float64(fanSpeed))

		if gpuMem, err = device.GetMemoryInfo(); err != nil {
			goto ErrorFetching
		}
		e.gauges["memory_free"].WithLabelValues(id, device.DeviceUUID, device.DeviceName).Set(float64(gpuMem.Free))
		e.gauges["memory_total"].WithLabelValues(id, device.DeviceUUID, device.DeviceName).Set(float64(gpuMem.Total))
		e.gauges["memory_used"].WithLabelValues(id, device.DeviceUUID, device.DeviceName).Set(float64(gpuMem.Used))
		continue

	ErrorFetching:
		log.Printf("Failed to query device %s: %s\n", device.DeviceUUID, err.Error())
		e.up.Set(0)
		return
	}
}

// Collect grabs the telemetry data from this machine using NVIDIA's Management Library.
// It implements prometheus.Collector.
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	for _, vec := range e.gauges {
		vec.Reset()
	}

	defer func() { ch <- e.up }()

	// If we fail at any point in retrieving GPU status, we fail 0
	e.up.Set(1)

	e.GetTelemetryFromNVML()

	for _, vec := range e.gauges {
		vec.Collect(ch)
	}
}

func init() {
	flag.StringVar(&listenAddress, "web.listen-address", ":9114", "Address to listen on")
	flag.StringVar(&metricsPath, "web.telemetry-path", "/metrics", "Path under which to expose metrics.")
	flag.Parse()
}

func main() {
	if err := InitNVML(); err != nil {
		log.Fatalf("Failed initializing exporter: %s\n", err.Error())
	}
	defer ShutdownNVML()

	landingPageHTML := []byte(fmt.Sprintf(`<html>
             <head><title>NVML Exporter</title></head>
             <body>
             <h1>NVML Exporter</h1>
             <p><a href='%s'>Metrics</a></p>
             </body>
             </html>`, metricsPath))

	log.Printf("Starting NVML Exporter Server: %s\n", listenAddress)
	exporter, err := NewExporter()
	if err != nil {
		log.Fatalf("Failed initializing exporter: %s\n", err.Error())
	}
	prometheus.MustRegister(exporter)

	http.Handle(metricsPath, prometheus.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write(landingPageHTML)
	})
	log.Fatal(http.ListenAndServe(listenAddress, nil))
}
