package daemon

import "sync"

type RepoLockManager struct {
	mu    sync.Mutex
	locks map[string]chan struct{}
}

func NewRepoLockManager() *RepoLockManager {
	return &RepoLockManager{locks: map[string]chan struct{}{}}
}

func (m *RepoLockManager) TryWithLock(repoPath string, fn func() error) (bool, error) {
	lock := m.get(repoPath)

	select {
	case <-lock:
		defer func() { lock <- struct{}{} }()
		return true, fn()
	default:
		return false, nil
	}
}

func (m *RepoLockManager) get(repoPath string) chan struct{} {
	m.mu.Lock()
	defer m.mu.Unlock()

	lock, ok := m.locks[repoPath]
	if !ok {
		lock = make(chan struct{}, 1)
		lock <- struct{}{}
		m.locks[repoPath] = lock
	}

	return lock
}
