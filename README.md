# NVIDIA GPU Telemetry Exporter for Prometheus

## Requirements

* CUDA 8.X - Modifications may need to be made in nvml.go to point to your install

## Building

    $ CGO_LDFLAGS="</usr/lib/nvidia-<driver_version>" go build -o nvidia_exporter

## Usage

    $ ./nvidia_exporter [flags]

### Flags

* __`web.listen-address`:__ Listen on this address for requests (default: `":9114"`).
* __`web.telemetry-path`:__ Path under which to expose metrics (default: `"/metrics"`).

## License

Portions of nvml.go are based on David Ressman [go-nvml](https://github.com/davidr/go-nvml). It has been cleaned up and modified for this exporter.

MIT