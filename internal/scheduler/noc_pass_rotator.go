package scheduler

import (
	"log"
	"sync"
	"time"

	"github.com/Flafl/DevOpsCore/internal/nocpass"
	"github.com/Flafl/DevOpsCore/internal/repository"
)

// NocPassRotator periodically rotates NOC passwords on enabled devices (24h cadence).
type NocPassRotator struct {
	repo     repository.NocPassRepository
	key      []byte
	stop     chan struct{}
	wg       sync.WaitGroup
	ticker   *time.Ticker
	stopOnce sync.Once
}

func NewNocPassRotator(repo repository.NocPassRepository, masterKey []byte) *NocPassRotator {
	return &NocPassRotator{
		repo: repo,
		key:  masterKey,
		stop: make(chan struct{}),
	}
}

func (r *NocPassRotator) Start() {
	r.ticker = time.NewTicker(15 * time.Minute)
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		r.runDue()
		for {
			select {
			case <-r.stop:
				return
			case <-r.ticker.C:
				r.runDue()
			}
		}
	}()
	log.Println("[noc-pass-rotator] started (check every 15m, rotate after 24h)")
}

func (r *NocPassRotator) Stop() {
	r.stopOnce.Do(func() {
		if r.ticker != nil {
			r.ticker.Stop()
		}
		close(r.stop)
		r.wg.Wait()
	})
}

func (r *NocPassRotator) runDue() {
	list, err := r.repo.ListEnabled()
	if err != nil {
		log.Printf("[noc-pass-rotator] list devices: %v", err)
		return
	}
	for i := range list {
		d := &list[i]
		if !nocpass.ShouldRotate(d) {
			continue
		}
		if err := nocpass.RotateAndApply(r.repo, r.key, d.ID); err != nil {
			log.Printf("[noc-pass-rotator] device id=%d host=%s: %v", d.ID, d.Host, err)
		}
	}
}
