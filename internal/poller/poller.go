// Package poller manages bounded, cancellable background tasks.
package poller

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"
)

const MaxTasks = 128

type task struct {
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}
type Pool struct {
	mu       sync.Mutex
	tasks    map[uintptr]*task
	stopping int
	io       chan struct{}
}

func New() *Pool { return &Pool{tasks: make(map[uintptr]*task), io: make(chan struct{}, 8)} }
func (p *Pool) Start(key uintptr, run func(<-chan struct{})) error {
	return p.StartContext(context.Background(), key, func(ctx context.Context) { run(ctx.Done()) })
}

// StartContext inherits the attachment lifetime. Completed tasks remove their
// own registration, while Stop keeps the key reserved until the worker exits.
func (p *Pool) StartContext(parent context.Context, key uintptr, run func(context.Context)) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := parent.Err(); err != nil {
		return err
	}
	if p.stopping > 0 {
		return fmt.Errorf("background tasks are stopping")
	}
	if _, ok := p.tasks[key]; ok {
		return fmt.Errorf("0x%x is already active", key)
	}
	if len(p.tasks) >= MaxTasks {
		return fmt.Errorf("background task limit (%d) reached", MaxTasks)
	}
	ctx, cancel := context.WithCancel(parent)
	t := &task{ctx: ctx, cancel: cancel, done: make(chan struct{})}
	p.tasks[key] = t
	go func() {
		defer func() { cancel(); p.mu.Lock(); delete(p.tasks, key); close(t.done); p.mu.Unlock() }()
		run(ctx)
	}()
	return nil
}
func (p *Pool) Stop(key uintptr) error {
	p.mu.Lock()
	t, ok := p.tasks[key]
	p.mu.Unlock()
	if !ok {
		return fmt.Errorf("0x%x is not active", key)
	}
	t.cancel()
	<-t.done
	return nil
}
func (p *Pool) StopAll() {
	p.mu.Lock()
	p.stopping++
	tasks := make([]*task, 0, len(p.tasks))
	for _, t := range p.tasks {
		t.cancel()
		tasks = append(tasks, t)
	}
	p.mu.Unlock()
	for _, t := range tasks {
		<-t.done
	}
	p.mu.Lock()
	p.stopping--
	p.mu.Unlock()
}
func (p *Pool) Has(key uintptr) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, ok := p.tasks[key]
	return ok
}
func (p *Pool) List() []uintptr {
	p.mu.Lock()
	defer p.mu.Unlock()
	keys := make([]uintptr, 0, len(p.tasks))
	for k := range p.tasks {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// Every bounds simultaneous I/O and checks cancellation again after a tick or
// a semaphore wait, so cancellation cannot schedule a final unwanted write.
func (p *Pool) Every(ctx context.Context, interval time.Duration, onTick func()) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		select {
		case <-ctx.Done():
			return
		case p.io <- struct{}{}:
		}
		if ctx.Err() == nil {
			onTick()
		}
		<-p.io
	}
}
func EveryTick(interval time.Duration, stop <-chan struct{}, onTick func()) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
		}
		select {
		case <-stop:
			return
		default:
			onTick()
		}
	}
}
