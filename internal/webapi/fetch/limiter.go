package fetch

import "sync"

// Limiter bounds in-flight Fetch operations shared by a browser session.
type Limiter struct {
	mu            sync.Mutex
	limit, active int
}

func NewLimiter(limit int) *Limiter { return &Limiter{limit: limit} }
func (limiter *Limiter) Acquire() bool {
	if limiter == nil {
		return true
	}
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	if limiter.limit > 0 && limiter.active >= limiter.limit {
		return false
	}
	limiter.active++
	return true
}
func (limiter *Limiter) Release() {
	if limiter == nil {
		return
	}
	limiter.mu.Lock()
	if limiter.active > 0 {
		limiter.active--
	}
	limiter.mu.Unlock()
}
