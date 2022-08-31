package golb

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"sync/atomic"
)

const (
	Attempts int = iota
	Retry
)

type Backend struct {
	URL          *url.URL
	Alive        bool
	syncc        sync.RWMutex
	ReverseProxy *httputil.ReverseProxy
}

// ServerPool is design to track all the backends in our load balancer
type ServerPool struct {
	backends []*Backend
	current  uint64
}

// NextIndex increases the current value by one atomically and returns
// the index by modding with the length of the slice, meaning the value
// always will be between 0 and length of the slice.
func (pool *ServerPool) NextIndex() int {
	return int(atomic.AddUint64(&pool.current, uint64(1)) % uint64(len(pool.backends)))
}

// GetNextPeer returns the next active peer to take a connection
func (pool *ServerPool) GetNextPeer() *Backend {
	next := pool.NextIndex()
	l := len(pool.backends) + next

	for i := next; i < l; i++ {
		idx := i % len(pool.backends)

		if pool.backends[idx].IsAlive() {
			if i != next {
				atomic.StoreUint64(&pool.current, uint64(idx))
			}
			return pool.backends[idx]
		}
	}
	return nil
}

// SetAlive for the backend in context
func (backend *Backend) SetAlive(alive bool) {
	backend.syncc.Lock()
	backend.Alive = alive
	backend.syncc.Unlock()
}

// IsAlive returns true when the backend in context is alive
func (backend *Backend) IsAlive() (alive bool) {
	backend.syncc.RLock()
	alive = backend.Alive
	backend.syncc.RUnlock()
	return
}

// AddBackend to the server pool
func (pool *ServerPool) AddBackend(backend *Backend) {
	pool.backends = append(pool.backends, backend)
}

// ChangeBackendStatus changes a status of a backend
func (pool *ServerPool) ChangeBackendStatus(backendUrl *url.URL, alive bool) {
	for _, b := range pool.backends {
		if b.URL.String() == backendUrl.String() {
			b.SetAlive(alive)
			break
		}
	}
}

func LB(rw http.ResponseWriter, req *http.Request) {
	peer := serverPool.GetNextPeer()
	if peer != nil {
		peer.ReverseProxy.ServeHTTP(rw, req)
		return
	}

	http.Error(rw, "Service not available", http.StatusServiceUnavailable)
}
