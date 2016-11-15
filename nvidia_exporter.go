package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

type VecInfo struct {
	help   string
	labels []string
}

var (
	DefaultNamespace = "nvml"
	// unexported variables below
	listenAddress string
	metricsPath   string

	gaugeMetrics = map[string]*VecInfo{
		"power_watts": &VecInfo{
			help:   "Power Usage of an NVIDIA GPU in Watts",
			labels: []string{"device_uuid", "device_name"},
		},
		"gpu_percent": &VecInfo{
			help:   "Percent of GPU Utilized",
			labels: []string{"device_uuid", "device_name"},
		},
		"memory_percent": &VecInfo{
			help:   "Percent of GPU Memory Utilized",
			labels: []string{"device_uuid", "device_name"},
		},
		"temperature_fahrenheit": &VecInfo{
			help:   "GPU Temperature in Fahrenheit",
			labels: []string{"device_uuid", "device_name"},
		},
		"temperature_celsius": &VecInfo{
			help:   "GPU Temperature in Celsius",
			labels: []string{"device_uuid", "device_name"},
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
func NewExporter() (exp *Exporter, err error) {
	exp = &Exporter{
		gauges: make(map[string]*prometheus.GaugeVec, len(gaugeMetrics)),
		up: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: DefaultNamespace,
			Name:      "up",
			Help:      "Were the NVML queries successful?",
		}),
	}

	if exp.devices, err = GetDevices(); err != nil {
		return
	}

	for name, info := range gaugeMetrics {
		exp.gauges[name] = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: DefaultNamespace,
			Name:      name,
			Help:      info.help,
		}, info.labels)
	}
	return
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
	for _, device := range e.devices {
		gpuPercent, memoryPercent, err := device.GetUtilization()
		if err != nil {
			fmt.Printf("Failed to get Device Utilization for %s: %s\n", device.DeviceUUID, err.Error())
			e.up.Set(0)
			return
		}
		e.gauges["gpu_percent"].WithLabelValues(device.DeviceUUID, device.DeviceName).Set(float64(gpuPercent))
		e.gauges["memory_percent"].WithLabelValues(device.DeviceUUID, device.DeviceName).Set(float64(memoryPercent))

		tempF, tempC, err := device.GetTemperature()
		if err != nil {
			fmt.Printf("Failed to get Device Temperature for %s: %s\n", device.DeviceUUID, err.Error())
			e.up.Set(0)
			return
		}
		e.gauges["temperature_celsius"].WithLabelValues(device.DeviceUUID, device.DeviceName).Set(float64(tempC))
		e.gauges["temperature_fahrenheit"].WithLabelValues(device.DeviceUUID, device.DeviceName).Set(float64(tempF))

		powerUsage, err := device.GetPowerUsage()
		if err != nil {
			fmt.Printf("Failed to get Device Power Usage for %s: %s\n", device.DeviceUUID, err.Error())
			e.up.Set(0)
			return
		}
		e.gauges["power_watts"].WithLabelValues(device.DeviceUUID, device.DeviceName).Set(float64(powerUsage))
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
}

func main() {
	flag.Parse()

	if err := InitNVML(); err != nil {
		log.Fatalf("Failed initializing exporter: %s\n", err.Error())
		return
	}
	defer ShutdownNVML()

	landingPageHTML := []byte(`<html>
             <head><title>NVML Exporter</title></head>
             <body>
             <h1>NVML Exporter</h1>
             <p><a href='` + metricsPath + `'>Metrics</a></p>
             </body>
             </html>`)

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
