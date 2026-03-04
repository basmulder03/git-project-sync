package daemon

import (
	"sync"
	"testing"
	"time"
)

func TestRepoLockManagerPreventsConcurrentExecution(t *testing.T) {
	t.Parallel()

	locks := NewRepoLockManager()
	start := make(chan struct{})
	release := make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		acquired, err := locks.TryWithLock("/repos/a", func() error {
			close(start)
			<-release
			return nil
		})
		if err != nil || !acquired {
			t.Errorf("first lock acquire failed: acquired=%t err=%v", acquired, err)
		}
	}()

	<-start
	acquired, err := locks.TryWithLock("/repos/a", func() error { return nil })
	if err != nil {
		t.Fatalf("second lock attempt returned error: %v", err)
	}
	if acquired {
		t.Fatal("expected second lock attempt to be skipped")
	}

	close(release)
	wg.Wait()

	acquired, err = locks.TryWithLock("/repos/a", func() error { return nil })
	if err != nil || !acquired {
		t.Fatalf("expected lock to be available after release: acquired=%t err=%v", acquired, err)
	}
}

func TestRepoLockManagerDifferentReposCanRun(t *testing.T) {
	t.Parallel()

	locks := NewRepoLockManager()
	begin := time.Now()

	var wg sync.WaitGroup
	for _, repo := range []string{"/repos/a", "/repos/b"} {
		repo := repo
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = locks.TryWithLock(repo, func() error {
				time.Sleep(30 * time.Millisecond)
				return nil
			})
		}()
	}
	wg.Wait()

	if time.Since(begin) > 80*time.Millisecond {
		t.Fatal("different repo locks should not serialize execution")
	}
}
