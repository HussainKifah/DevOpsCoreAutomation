package shell

import (
	"fmt"
	"strings"
	"sync"
	"time"

	probing "github.com/prometheus-community/pro-bing"
)

func PingNocDataHost(ip string, timeout time.Duration) error {
	pinger, err := probing.NewPinger(strings.TrimSpace(ip))
	if err != nil {
		return fmt.Errorf("new pinger: %w", err)
	}

	pinger.Count = 1
	pinger.Timeout = timeout
	pinger.SetPrivileged(false)

	if err := pinger.Run(); err != nil {
		return fmt.Errorf("run pinger: %w", err)
	}

	stats := pinger.Statistics()
	if stats.PacketsRecv == 0 {
		return fmt.Errorf("no reply from %s", ip)
	}

	return nil
}

func FilterReachableNocDataHosts(hosts []string, timeout time.Duration, workers int) []string {
	if workers < 1 {
		workers = 1
	}

	jobs := make(chan string)
	reachable := make(chan string, len(hosts))
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for host := range jobs {
				if err := PingNocDataHost(host, timeout); err == nil {
					reachable <- host
				}
			}
		}()
	}

	go func() {
		for _, host := range hosts {
			jobs <- host
		}
		close(jobs)
		wg.Wait()
		close(reachable)
	}()

	out := make([]string, 0, len(hosts))
	seen := make(map[string]struct{}, len(hosts))
	for host := range reachable {
		seen[host] = struct{}{}
	}
	for _, host := range hosts {
		if _, ok := seen[host]; ok {
			out = append(out, host)
		}
	}

	return out
}
