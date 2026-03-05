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

func TestRepoLockManagerContentionSpikeDoesNotPoisonLock(t *testing.T) {
	t.Parallel()

	locks := NewRepoLockManager()

	const rounds = 30
	const contenders = 25

	for round := 0; round < rounds; round++ {
		holderReady := make(chan struct{})
		releaseHolder := make(chan struct{})
		holderDone := make(chan struct{})

		go func() {
			defer close(holderDone)
			acquired, err := locks.TryWithLock("/repos/hot", func() error {
				close(holderReady)
				<-releaseHolder
				return nil
			})
			if err != nil {
				t.Errorf("holder lock error: %v", err)
				return
			}
			if !acquired {
				t.Error("holder failed to acquire lock")
			}
		}()

		<-holderReady

		start := make(chan struct{})
		var wg sync.WaitGroup
		var acquiredCount int
		var mu sync.Mutex

		for i := 0; i < contenders; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				acquired, err := locks.TryWithLock("/repos/hot", func() error {
					time.Sleep(2 * time.Millisecond)
					return nil
				})
				if err != nil {
					t.Errorf("unexpected lock error: %v", err)
					return
				}
				if acquired {
					mu.Lock()
					acquiredCount++
					mu.Unlock()
				}
			}()
		}

		close(start)
		wg.Wait()

		if acquiredCount != 0 {
			t.Fatalf("round %d contender acquired count = %d, want 0 while holder lock is active", round, acquiredCount)
		}

		close(releaseHolder)
		<-holderDone
	}

	acquired, err := locks.TryWithLock("/repos/hot", func() error { return nil })
	if err != nil || !acquired {
		t.Fatalf("lock should remain usable after contention spike: acquired=%t err=%v", acquired, err)
	}
}
