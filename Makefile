run:
	go run main.go

dog:
	docker run -d --name dd-agent -v /var/run/docker.sock:/var/run/docker.sock:ro -v /proc/:/host/proc/:ro -v /sys/fs/cgroup/:/host/sys/fs/cgroup:ro -e DD_API_KEY=$(DD_API_KEY) -p 8125:8125/udp datadog/agent:latest
