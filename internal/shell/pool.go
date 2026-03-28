package shell

import (
	"log"
	"regexp"
	"sync"
	"time"

	"github.com/scrapli/scrapligo/driver/generic"
	"github.com/scrapli/scrapligo/driver/options"
	"github.com/scrapli/scrapligo/transport"
)

type poolEntry struct {
	driver *generic.Driver
	mu     sync.Mutex
}

type ConnectionPool struct {
	user    string
	pass    string
	mu      sync.RWMutex
	conns   map[string]*poolEntry
	stopped chan struct{}
}

func NewConnectionPool(user, pass string) *ConnectionPool {
	p := &ConnectionPool{
		user:    user,
		pass:    pass,
		conns:   make(map[string]*poolEntry),
		stopped: make(chan struct{}),
	}
	go p.keepAliveLoop()
	return p
}

func (p *ConnectionPool) getOrConnect(host string) (*poolEntry, error) {
	p.mu.RLock()
	entry, ok := p.conns[host]
	p.mu.RUnlock()
	if ok {
		return entry, nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if entry, ok = p.conns[host]; ok {
		return entry, nil
	}

	d, err := p.dial(host)
	if err != nil {
		return nil, err
	}

	entry = &poolEntry{driver: d}
	p.conns[host] = entry
	log.Printf("[pool] connected to %s", host)
	return entry, nil
}

func (p *ConnectionPool) dial(host string) (*generic.Driver, error) {
	d, err := generic.NewDriver(
		host,
		options.WithAuthNoStrictKey(),
		options.WithAuthUsername(p.user),
		options.WithAuthPassword(p.pass),
		options.WithPromptPattern(regexp.MustCompile(`(?m)(>#)\s*$`)),
		options.WithTransportType(transport.StandardTransport),
		options.WithSSHConfigFile(""),
		options.WithTimeoutSocket(60*time.Second),
		options.WithStandardTransportExtraKexs(scrapligoWideKEX),
		options.WithStandardTransportExtraCiphers(scrapligoWideCiphers),
		options.WithTimeoutOps(120*time.Minute),
		options.WithTermWidth(500),
	)
	if err != nil {
		return nil, err
	}
	if err := d.Open(); err != nil {
		return nil, err
	}
	return d, nil
}

func (p *ConnectionPool) SendCommand(host string, cmds ...string) (string, error) {
	entry, err := p.getOrConnect(host)
	if err != nil {
		return "", err
	}

	entry.mu.Lock()
	defer entry.mu.Unlock()

	result, err := p.sendOnDriver(entry.driver, cmds...)
	if err != nil {
		log.Printf("[pool] command failed on %s, reconnecting: %v", host, err)
		entry.driver.Close()
		d, dialErr := p.dial(host)
		if dialErr != nil {
			p.mu.Lock()
			delete(p.conns, host)
			p.mu.Unlock()
			return "", dialErr
		}
		entry.driver = d
		log.Printf("[pool] reconnected to %s", host)
		result, err = p.sendOnDriver(entry.driver, cmds...)
	}
	return result, err
}

func (p *ConnectionPool) sendOnDriver(d *generic.Driver, cmds ...string) (string, error) {
	if len(cmds) == 1 {
		r, err := d.SendCommand(cmds[0])
		if err != nil {
			return "", err
		}
		return r.Result, nil
	}
	rs, err := d.SendCommands(cmds)
	if err != nil {
		return "", err
	}
	return rs.JoinedResult(), nil
}

func (p *ConnectionPool) keepAliveLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			p.mu.RLock()
			hosts := make([]string, 0, len(p.conns))
			for h := range p.conns {
				hosts = append(hosts, h)
			}
			p.mu.RUnlock()

			for _, host := range hosts {
				p.mu.RLock()
				entry, ok := p.conns[host]
				p.mu.RUnlock()
				if !ok {
					continue
				}
				entry.mu.Lock()
				_, err := entry.driver.SendCommand("")
				if err != nil {
					log.Printf("[pool] keepalive failed for %s, removing", host)
					entry.driver.Close()
					p.mu.Lock()
					delete(p.conns, host)
					p.mu.Unlock()
				}
				entry.mu.Unlock()
			}
		case <-p.stopped:
			return
		}
	}
}

func (p *ConnectionPool) Close() {
	close(p.stopped)
	p.mu.Lock()
	defer p.mu.Unlock()
	for host, entry := range p.conns {
		entry.driver.Close()
		delete(p.conns, host)
	}
	log.Println("[pool] all connections closed")
}

func SendCommandNokiaOLTsPooled(pool *ConnectionPool, cmds ...string) <-chan Result {
	nokia, _, err := OLTsData()
	if err != nil {
		log.Printf("Failed to fetch OLT data: %v", err)
		ch := make(chan Result, 1)
		ch <- Result{Err: err}
		close(ch)
		return ch
	}
	results := make(chan Result, len(nokia))
	var wg sync.WaitGroup
	sem := make(chan struct{}, len(nokia)) // allow all OLTs to run concurrently

	for _, olt := range nokia {
		olt := olt
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			data, err := pool.SendCommand(olt.Ip, cmds...)
			if err != nil {
				// Retry once for transient failures (no stagger - all still run in parallel)
				time.Sleep(2 * time.Second)
				data, err = pool.SendCommand(olt.Ip, cmds...)
			}
			results <- Result{Device: olt.Name, Site: olt.Site, Host: olt.Ip, Data: data, Err: err}
		}()
	}
	go func() {
		wg.Wait()
		close(results)
	}()
	return results
}
