<img src="doc/auklet.png" alt="Auklet Logo" height="100">

# Auklet
An autoscaler for Docker Swarm that scales the number of replicas for a
Service based on service labels.

# Usage
Auklet binds to Docker using the socket or tcp endpoint using the same environment
variables and/or defaults as the Docker client. Specifically `DOCKER_HOST`.

Usage of the `auklet` binary is self explanatory. Use `auklet --help` for more
information. Auklet as a minimum needs the `-p` flag to specify the endpoint
of the Prometheus instance.

For a service to be monitored by `auklet`, a number of labels need to be set
when creating the service:

| label | required | type | default | description |
| ----- | -------- | ---- | ------- | ----------- |
| auklet.autoscale | * | bool | - | set to true to enable autoscaling by auklet |
| auklet.scale_min | * | int | - | minimum number of replicas the service can have |
| auklet.scale_max | * | int | - | maximum number of replicas the service can have |
| auklet.up_step | - | int | 1 | number of replicas to be added when scaling up |
| auklet.down_step | - | int | 1 | number of replicas to be removed when scaling down |
| auklet.query | * | string | - | the PromQL query to get the metric used for the scaling decision |
| auklet.up_threshold | * | float64 | - | upper threshold the queried metric is tested against |
| auklet.down_threshold | * | float64 | - | lower threshold the queries metric is tested against |
| auklet.up_graceperiod | - | duration | 0s | duration the metric is allowed to be above upper threshold before actually scaling up |
| auklet.down_graceperiod | - | duration | 0s | duration the metric is allowed to be below lower threshold before actually scaling down |

If Auklet fails to find and parse the required labels, an error will be issued
and the service will not be monitored (ignored) by Auklet. When services are
added, updated or removed Auklet will automatically retry to monitor the service.

# HTTP endpoints
Auklet exposes pprof and metrics endpoints for profiling and metrics collection:
- `/debug/pprof/`; for profiling
- `/metrics`; for metrics scraping (Prometheus)

# Disclaimer
Auklet is a personal toy project, a work in progress, and nowhere near production
ready. I'd love to get it to production quality, but in the meantime use at your
own risk!

# Build
See the included `Makefile` to see building, testing and checking options.
For a local, non-Dockerized build use `make localbuild`. For building a linux
binary (using Docker), simply use `make`.

# Contributing
If you like this project and would like to contribute, simply file an issue 
and/or submit a PR. Please make sure all tests pass, and `make check` returns
empty before submitting the PR.
