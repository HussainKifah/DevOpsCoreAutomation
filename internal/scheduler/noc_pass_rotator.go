package scheduler

import (
	"log"
	"sync"
	"time"

	"github.com/Flafl/DevOpsCore/internal/nocpass"
	"github.com/Flafl/DevOpsCore/internal/repository"
)

// NocPassRotator periodically applies enabled NOC PASS policies.
type NocPassRotator struct {
	repo        repository.NocPassRepository
	nocDataRepo repository.NocDataRepository
	key         []byte
	stop        chan struct{}
	wg          sync.WaitGroup
	ticker      *time.Ticker
	stopOnce    sync.Once
}

func NewNocPassRotator(repo repository.NocPassRepository, nocDataRepo repository.NocDataRepository, masterKey []byte) *NocPassRotator {
	return &NocPassRotator{
		repo:        repo,
		nocDataRepo: nocDataRepo,
		key:         masterKey,
		stop:        make(chan struct{}),
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
	log.Println("[noc-pass-rotator] started (check every 15m, apply due enabled policies)")
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
	policies, err := r.repo.ListPolicies()
	if err != nil {
		log.Printf("[noc-pass-rotator] list policies: %v", err)
		return
	}
	now := time.Now()
	for i := range policies {
		policy := &policies[i]
		if !nocpass.ShouldRunPolicy(policy, now) {
			continue
		}
		if _, err := nocpass.RunPolicy(r.repo, r.nocDataRepo, r.key, policy.ID, now); err != nil {
			log.Printf("[noc-pass-rotator] run policy id=%d: %v", policy.ID, err)
		}
	}
}
