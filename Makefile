run:
	go run main.go

dog:
	docker run --rm -d --name dd-agent -v /var/run/docker.sock:/var/run/docker.sock:ro -v /proc/:/host/proc/:ro -v /sys/fs/cgroup/:/host/sys/fs/cgroup:ro -e DD_API_KEY=$(DD_API_KEY) -e DD_DOGSTATSD_NON_LOCAL_TRAFFIC=true -p 8125:8125/udp datadog/agent:latest
